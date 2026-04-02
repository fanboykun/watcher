package agent

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

// RepoWatcher manages the poll loop for a single watcher entry from the database.
// Multiple RepoWatchers run concurrently inside the main Agent.
type RepoWatcher struct {
	wcfg      *WatcherConfig // converted from DB model
	global    *config.AppConfig
	log       *Logger
	state     *StateManager
	deployer  *Deployer
	db        *gorm.DB
	watcherID uint
}

// WatcherConfig is the in-memory representation used by the deploy pipeline.
// Constructed from a database.Watcher + its database.Service records.
type WatcherConfig struct {
	Name                  string
	ServiceName           string
	MetadataURL           string
	ReleaseRef            string
	DeploymentEnvironment string
	GitHubToken           string
	CheckIntervalSec      int
	DownloadRetries       int
	InstallDir            string
	Paused                bool
	MaxKeptVersions       int
	HealthCheck           HealthCheckConfig
	Services              []ServiceConfig
}

type HealthCheckConfig struct {
	Enabled     bool
	URL         string
	Retries     int
	IntervalSec int
	TimeoutSec  int
}

type ServiceConfig struct {
	ServiceType        string // "nssm" or "static"
	WindowsServiceName string
	BinaryName         string
	StartArguments     string
	EnvFile            string
	HealthCheckURL     string
	IISAppPool         string
	IISSiteName        string
}

// WatcherConfigFromDB converts a database.Watcher into the in-memory WatcherConfig
// used by the deploy pipeline.
func WatcherConfigFromDB(w *database.Watcher) *WatcherConfig {
	releaseRef := strings.TrimSpace(w.ReleaseRef)
	if releaseRef == "" {
		releaseRef = "latest"
	}
	cfg := &WatcherConfig{
		Name:                  w.Name,
		ServiceName:           w.ServiceName,
		MetadataURL:           w.MetadataURL,
		ReleaseRef:            releaseRef,
		DeploymentEnvironment: strings.TrimSpace(w.DeploymentEnvironment),
		GitHubToken:           strings.TrimSpace(w.GitHubToken),
		CheckIntervalSec:      w.CheckIntervalSec,
		DownloadRetries:       w.DownloadRetries,
		InstallDir:            w.InstallDir,
		Paused:                w.Paused,
		MaxKeptVersions:       max(w.MaxKeptVersions, 1),
		HealthCheck: HealthCheckConfig{
			Enabled:     w.HcEnabled,
			URL:         w.HcURL,
			Retries:     w.HcRetries,
			IntervalSec: w.HcIntervalSec,
			TimeoutSec:  w.HcTimeoutSec,
		},
	}
	for _, s := range w.Services {
		svcType := s.ServiceType
		if svcType == "" {
			svcType = "nssm" // default for backwards compatibility
		}
		cfg.Services = append(cfg.Services, ServiceConfig{
			ServiceType:        svcType,
			WindowsServiceName: s.WindowsServiceName,
			BinaryName:         s.BinaryName,
			StartArguments:     s.StartArguments,
			EnvFile:            s.EnvFile,
			HealthCheckURL:     s.HealthCheckURL,
			IISAppPool:         s.IISAppPool,
			IISSiteName:        s.IISSiteName,
		})
	}
	return cfg
}

func NewRepoWatcher(dbWatcher *database.Watcher, db *gorm.DB, appCfg *config.AppConfig, log *Logger, events *WatcherEventBus) *RepoWatcher {
	wcfg := WatcherConfigFromDB(dbWatcher)
	componentLog := log.WithComponent(wcfg.Name)
	state := NewStateManager(db, dbWatcher.ID, componentLog, events)
	return &RepoWatcher{
		wcfg:      wcfg,
		global:    appCfg,
		log:       componentLog,
		state:     state,
		deployer:  NewDeployer(wcfg, appCfg.NssmPath, componentLog, state.AppendDeployLog),
		db:        db,
		watcherID: dbWatcher.ID,
	}
}

// Run performs one check-and-deploy cycle for this repo.
func (r *RepoWatcher) Run(ctx context.Context) error {
	if r.wcfg.Paused {
		r.log.Debug("watcher is paused, skipping check")
		return nil
	}

	r.log.Info("check cycle", "metadata_url", r.wcfg.MetadataURL)

	_ = r.state.SetChecked()

	gh := NewGitHubClient(r.resolveGitHubToken(), r.log)
	meta, err := gh.FetchServiceMetadataForRelease(ctx, r.wcfg.MetadataURL, r.wcfg.ReleaseRef, r.wcfg.ServiceName)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			r.state.RecordPollEvent("error", "", err.Error())
		}
		return fmt.Errorf("fetch metadata: %w", err)
	}

	svcMeta, ok := meta.Services[r.wcfg.ServiceName]
	if !ok {
		err := fmt.Errorf("service %q not found in release metadata (available: %v)", r.wcfg.ServiceName, keys(meta.Services))
		r.state.RecordPollEvent("error", "", err.Error())
		return err
	}

	targetVersion := svcMeta.Version
	r.log.Info("remote version", "target", targetVersion, "published_at", svcMeta.PublishedAt)

	localVersion, maxIgnoredVersion, err := r.state.ReadVersion()
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			r.state.RecordPollEvent("error", targetVersion, "read local version: "+err.Error())
		}
		return fmt.Errorf("read local version: %w", err)
	}
	r.log.Info("local version", "current", localVersion, "max_ignored", maxIgnoredVersion)

	if localVersion == targetVersion {
		r.log.Info("already up to date")
		r.state.RecordPollEvent("no_update", targetVersion, "")
		return nil
	}

	if maxIgnoredVersion != "" && !isNewer(targetVersion, maxIgnoredVersion) {
		r.log.Info("update skipped due to rollback high-watermark", "target", targetVersion, "ignored_upto", maxIgnoredVersion)
		r.state.RecordPollEvent("skipped", targetVersion, fmt.Sprintf("skipped (<= %s) because of manual rollback", maxIgnoredVersion))
		return nil
	}

	r.state.RecordPollEvent("new_release", targetVersion, "")
	r.log.Info("version mismatch, deploying", "from", localVersion, "to", targetVersion)

	// Prevent infinite deploy retries — cap at 3 consecutive failures for the same version.
	// A manual redeploy from the dashboard bypasses this once by pre-creating an open manual deploy log.
	const maxDeployRetries = 3
	failures := r.state.ConsecutiveFailuresForVersion(targetVersion)
	manualRedeploy := r.state.HasPendingManualDeploy()
	if failures >= maxDeployRetries && !manualRedeploy {
		msg := fmt.Sprintf("deploy suspended for %s after %d consecutive failures — use dashboard to redeploy", targetVersion, failures)
		r.log.Warn(msg)
		r.state.RecordPollEvent("deploy_suspended", targetVersion, msg)
		return nil
	}

	if err := r.deploy(ctx, gh, svcMeta, targetVersion, localVersion); err != nil {
		if !errors.Is(err, context.Canceled) {
			_ = r.state.SetFailed(err.Error())
		}
		return fmt.Errorf("deploy: %w", err)
	}

	return nil
}

func (r *RepoWatcher) deploy(ctx context.Context, gh *GitHubClient, svcMeta ServiceMeta, targetVersion, previousVersion string) error {
	deployLogID, _ := r.state.SetDeploying(targetVersion, previousVersion)

	// ── GitHub Deployment API integration (optional) ──────────────
	var ghDeploymentID int64
	var ghOwner, ghRepo string
	resolvedEnv := resolveDeploymentEnvironment(r.wcfg.DeploymentEnvironment, r.global.Environment)
	useGHDeploy := r.global.GitHubDeployEnabled && strings.TrimSpace(r.resolveGitHubToken()) != ""
	logURL := buildDeployLogURL(r.global.APIBaseURL, r.watcherID, deployLogID)
	r.state.AppendDeployLog(fmt.Sprintf("github_deployment: environment=%q", resolvedEnv))
	if !r.global.GitHubDeployEnabled {
		r.state.AppendDeployLog("github_deployment: disabled by GITHUB_DEPLOY_ENABLED=false")
	} else if !useGHDeploy {
		r.state.AppendDeployLog("github_deployment: disabled because GitHub token is empty (watcher + global)")
	}
	if useGHDeploy && resolvedEnv == "" {
		r.state.AppendDeployLog("github_deployment: disabled because deployment environment is empty (watcher + global ENVIRONMENT)")
		useGHDeploy = false
	}

	if useGHDeploy {
		var err error
		ghOwner, ghRepo, err = resolveDeploymentRepo(r.wcfg.MetadataURL, svcMeta.ArtifactURL)
		if err != nil {
			r.state.AppendDeployLog("github_deployment: parse repo failed: " + err.Error())
			r.log.Warn("cannot parse repo for GitHub Deployment API", "error", err)
			useGHDeploy = false
		}
	}

	if useGHDeploy {
		if logURL == "" && strings.TrimSpace(r.global.APIBaseURL) != "" {
			r.state.AppendDeployLog("github_deployment: invalid API_BASE_URL, skipping log_url in deployment statuses")
		}
		desc := fmt.Sprintf("Deploying %s %s", r.wcfg.ServiceName, targetVersion)
		deployRef := resolveDeploymentRef(targetVersion, svcMeta.ArtifactURL)
		r.state.AppendDeployLog(fmt.Sprintf("github_deployment: creating deployment repo=%s/%s ref=%s env=%q", ghOwner, ghRepo, deployRef, resolvedEnv))
		id, err := gh.CreateDeployment(ctx, ghOwner, ghRepo, deployRef, resolvedEnv, desc)
		if err != nil {
			r.state.AppendDeployLog("github_deployment: create deployment failed: " + err.Error())
			r.log.Warn("failed to create GitHub deployment", "error", err)
			useGHDeploy = false
		} else {
			ghDeploymentID = id
			_ = r.state.SetGitHubDeploymentID(deployLogID, ghDeploymentID)
			r.state.AppendDeployLog(fmt.Sprintf("github_deployment: created deployment id=%d", ghDeploymentID))
			r.state.AppendDeployLog("github_deployment: setting status=in_progress")
			if err := gh.UpdateDeploymentStatus(ctx, ghOwner, ghRepo, ghDeploymentID, "in_progress", logURL, desc); err != nil {
				r.state.AppendDeployLog("github_deployment: set status=in_progress failed: " + err.Error())
			}
		}
	}

	// ── Actual deploy ─────────────────────────────────────────────
	if err := os.MkdirAll(r.wcfg.InstallDir, 0755); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("create install dir: %w", err)
	}

	if svcMeta.ArtifactURL == "" {
		err := fmt.Errorf("artifact_url missing in version.json for %s", targetVersion)
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return err
	}

	downloadsDir := filepath.Join(r.wcfg.InstallDir, "downloads")
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("create downloads dir: %w", err)
	}

	zipPath := filepath.Join(downloadsDir, releaseStorageName(targetVersion)+".zip")
	if err := gh.DownloadArtifact(ctx, svcMeta.ArtifactURL, zipPath, r.wcfg.DownloadRetries); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return fmt.Errorf("download artifact: %w", err)
	}
	defer func() {
		r.log.Debug("removing zip", "path", zipPath)
		os.Remove(zipPath)
	}()

	if err := r.deployer.Deploy(ctx, targetVersion, zipPath, previousVersion); err != nil {
		r.ghDeployFailure(ctx, gh, useGHDeploy, ghOwner, ghRepo, ghDeploymentID, deployLogID, err.Error())
		return err
	}

	if err := r.state.WriteVersion(targetVersion); err != nil {
		r.log.Warn("failed to write version", "error", err)
	}
	if err := r.state.SetHealthy(targetVersion); err != nil {
		r.log.Warn("failed to update state", "error", err)
	}

	// ── GitHub Deployment: success ────────────────────────────────
	if useGHDeploy {
		r.state.AppendDeployLog("github_deployment: setting status=success")
		if err := gh.UpdateDeploymentStatus(ctx, ghOwner, ghRepo, ghDeploymentID, "success", logURL,
			fmt.Sprintf("Deployed %s %s", r.wcfg.ServiceName, targetVersion)); err != nil {
			r.state.AppendDeployLog("github_deployment: set status=success failed: " + err.Error())
		}
	}

	// ── Clean old releases ────────────────────────────────────────
	if err := CleanOldReleases(r.wcfg.InstallDir, r.wcfg.MaxKeptVersions); err != nil {
		r.log.Warn("failed to clean old releases", "error", err)
	}

	r.log.Info("deploy complete", "version", targetVersion)
	return nil
}

// ghDeployFailure updates the GitHub deployment status to "failure" if integration is active.
func (r *RepoWatcher) ghDeployFailure(ctx context.Context, gh *GitHubClient, active bool, owner, repo string, deploymentID int64, deployLogID uint, errMsg string) {
	if !active {
		return
	}
	logURL := buildDeployLogURL(r.global.APIBaseURL, r.watcherID, deployLogID)
	r.state.AppendDeployLog("github_deployment: setting status=failure")
	if err := gh.UpdateDeploymentStatus(ctx, owner, repo, deploymentID, "failure", logURL, errMsg); err != nil {
		r.state.AppendDeployLog("github_deployment: set status=failure failed: " + err.Error())
	}
}

func (r *RepoWatcher) resolveGitHubToken() string {
	if t := strings.TrimSpace(r.wcfg.GitHubToken); t != "" {
		return t
	}
	return strings.TrimSpace(r.global.GitHubToken)
}

// ── Agent — runs all RepoWatchers concurrently ────────────────────────────────

type watcherHandle struct {
	cancel    context.CancelFunc
	trigger   chan struct{}
	updatedAt int64
}

// Agent manages all RepoWatcher instances, one goroutine per watcher entry.
type Agent struct {
	db           *gorm.DB
	appCfg       *config.AppConfig
	log          *Logger
	events       *WatcherEventBus
	checkTrigger chan uint
	syncTrigger  chan struct{}

	mu       sync.Mutex
	watchers map[uint]watcherHandle
}

func NewAgent(db *gorm.DB, appCfg *config.AppConfig, log *Logger, events *WatcherEventBus, checkTrigger chan uint, syncTrigger chan struct{}) *Agent {
	return &Agent{
		db:           db,
		appCfg:       appCfg,
		log:          log,
		events:       events,
		checkTrigger: checkTrigger,
		syncTrigger:  syncTrigger,
		watchers:     make(map[uint]watcherHandle),
	}
}

// Run loads all watchers from the database, starts one goroutine per watcher,
// and blocks until ctx is cancelled.
func (a *Agent) Run(ctx context.Context) {
	// Initial load
	a.syncWatchers(ctx)

	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			for _, h := range a.watchers {
				h.cancel()
			}
			a.mu.Unlock()
			a.log.Info("all watchers stopped")
			return
		case <-a.syncTrigger:
			a.log.Info("syncing watchers from database")
			a.syncWatchers(ctx)
		case triggerID := <-a.checkTrigger:
			a.mu.Lock()
			h, ok := a.watchers[triggerID]
			a.mu.Unlock()
			if ok {
				select {
				case h.trigger <- struct{}{}:
				default:
				}
			} else {
				a.log.Warn("check trigger for unknown watcher", "id", triggerID)
			}
		}
	}
}

func (a *Agent) syncWatchers(ctx context.Context) {
	var dbWatchers []database.Watcher
	if err := a.db.Preload("Services").Find(&dbWatchers).Error; err != nil {
		a.log.Error("failed to load watchers from database", "error", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	currentIDs := make(map[uint]bool)
	for _, w := range dbWatchers {
		currentIDs[w.ID] = true
	}

	// Stop removed watchers
	for id, h := range a.watchers {
		if !currentIDs[id] {
			a.log.Info("stopping removed watcher", "id", id)
			h.cancel()
			delete(a.watchers, id)
		}
	}

	// Start or update watchers
	for i := range dbWatchers {
		w := &dbWatchers[i]
		h, exists := a.watchers[w.ID]

		updatedAt := w.UpdatedAt.UnixNano()
		if exists && h.updatedAt == updatedAt {
			continue // Unchanged
		}

		if exists {
			a.log.Info("restarting updated watcher", "id", w.ID)
			h.cancel()
		} else {
			a.log.Info("starting new watcher", "id", w.ID)
		}

		wCtx, cancel := context.WithCancel(ctx)
		trigger := make(chan struct{}, 1)
		a.watchers[w.ID] = watcherHandle{
			cancel:    cancel,
			trigger:   trigger,
			updatedAt: updatedAt,
		}

		go a.runWatcher(wCtx, w, trigger)
	}
}

func (a *Agent) runWatcher(ctx context.Context, dbWatcher *database.Watcher, trigger chan struct{}) {
	log := a.log.WithComponent(dbWatcher.Name)
	rw := NewRepoWatcher(dbWatcher, a.db, a.appCfg, a.log, a.events)

	log.Info("watcher starting",
		"service_name", dbWatcher.ServiceName,
		"metadata_url", dbWatcher.MetadataURL,
		"check_interval_sec", dbWatcher.CheckIntervalSec,
		"services", len(dbWatcher.Services),
	)

	// Run immediately on startup
	if err := rw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("initial check failed", "error", err)
	}

	ticker := newTicker(dbWatcher.CheckIntervalSec)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("watcher stopping")
			return
		case <-ticker.C:
			if err := rw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("check cycle failed", "error", err)
			}
		case <-trigger:
			log.Info("immediate check triggered via API")
			if err := rw.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("triggered check failed", "error", err)
			}
		}
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func resolveDeploymentRepo(metadataURL, artifactURL string) (owner, repo string, err error) {
	if artifactURL != "" {
		aOwner, aRepo, _, _, aErr := parseReleaseDownloadURL(artifactURL)
		if aErr == nil {
			return aOwner, aRepo, nil
		}
	}
	return ParseGitHubURL(metadataURL)
}

func resolveDeploymentRef(targetVersion, artifactURL string) string {
	if artifactURL != "" {
		_, _, tag, _, err := parseReleaseDownloadURL(artifactURL)
		if err == nil && strings.TrimSpace(tag) != "" {
			return tag
		}
	}
	return targetVersion
}

func resolveDeploymentEnvironment(watcherEnv, globalEnv string) string {
	if env := strings.TrimSpace(watcherEnv); env != "" {
		return env
	}
	return strings.TrimSpace(globalEnv)
}

func buildDeployLogURL(apiBaseURL string, watcherID, deployLogID uint) string {
	base := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if strings.EqualFold(u.Path, "/api") {
		u.Path = ""
		base = strings.TrimRight(u.String(), "/")
	}
	return fmt.Sprintf("%s/watchers/%d/logs/%d", base, watcherID, deployLogID)
}
