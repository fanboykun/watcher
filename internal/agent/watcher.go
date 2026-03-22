package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

// RepoWatcher manages the poll loop for a single watcher entry from the database.
// Multiple RepoWatchers run concurrently inside the main Agent.
type RepoWatcher struct {
	wcfg     *WatcherConfig // converted from DB model
	global   *config.AppConfig
	log      *Logger
	github   *GitHubClient
	state    *StateManager
	deployer *Deployer
}

// WatcherConfig is the in-memory representation used by the deploy pipeline.
// Constructed from a database.Watcher + its database.Service records.
type WatcherConfig struct {
	Name             string
	ServiceName      string
	MetadataURL      string
	CheckIntervalSec int
	DownloadRetries  int
	InstallDir       string
	HealthCheck      HealthCheckConfig
	Services         []ServiceConfig
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
	EnvFile            string
	HealthCheckURL     string
	IISAppPool         string
	IISSiteName        string
}

// WatcherConfigFromDB converts a database.Watcher into the in-memory WatcherConfig
// used by the deploy pipeline.
func WatcherConfigFromDB(w *database.Watcher) *WatcherConfig {
	cfg := &WatcherConfig{
		Name:             w.Name,
		ServiceName:      w.ServiceName,
		MetadataURL:      w.MetadataURL,
		CheckIntervalSec: w.CheckIntervalSec,
		DownloadRetries:  w.DownloadRetries,
		InstallDir:       w.InstallDir,
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
			EnvFile:            s.EnvFile,
			HealthCheckURL:     s.HealthCheckURL,
			IISAppPool:         s.IISAppPool,
			IISSiteName:        s.IISSiteName,
		})
	}
	return cfg
}

func NewRepoWatcher(dbWatcher *database.Watcher, db *gorm.DB, appCfg *config.AppConfig, log *Logger) *RepoWatcher {
	wcfg := WatcherConfigFromDB(dbWatcher)
	componentLog := log.WithComponent(wcfg.Name)
	return &RepoWatcher{
		wcfg:     wcfg,
		global:   appCfg,
		log:      componentLog,
		github:   NewGitHubClient(appCfg.GitHubToken, componentLog),
		state:    NewStateManager(db, dbWatcher.ID, componentLog),
		deployer: NewDeployer(wcfg, appCfg.NssmPath, componentLog),
	}
}

// Run performs one check-and-deploy cycle for this repo.
func (r *RepoWatcher) Run(ctx context.Context) error {
	r.log.Info("check cycle", "metadata_url", r.wcfg.MetadataURL)

	_ = r.state.SetChecked()

	meta, err := r.github.FetchMetadata(ctx, r.wcfg.MetadataURL)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}

	svcMeta, ok := meta.Services[r.wcfg.ServiceName]
	if !ok {
		return fmt.Errorf("service %q not in version.json (available: %v)", r.wcfg.ServiceName, keys(meta.Services))
	}

	targetVersion := svcMeta.Version
	r.log.Info("remote version", "target", targetVersion, "published_at", svcMeta.PublishedAt)

	localVersion, err := r.state.ReadVersion()
	if err != nil {
		return fmt.Errorf("read local version: %w", err)
	}
	r.log.Info("local version", "current", localVersion)

	if localVersion == targetVersion {
		r.log.Info("already up to date")
		return nil
	}

	r.log.Info("version mismatch, deploying", "from", localVersion, "to", targetVersion)

	if err := r.deploy(ctx, svcMeta, targetVersion, localVersion); err != nil {
		_ = r.state.SetFailed(err.Error())
		return fmt.Errorf("deploy: %w", err)
	}

	return nil
}

func (r *RepoWatcher) deploy(ctx context.Context, svcMeta ServiceMeta, targetVersion, previousVersion string) error {
	_ = r.state.SetDeploying(targetVersion, previousVersion)

	if err := os.MkdirAll(r.wcfg.InstallDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	if svcMeta.ArtifactURL == "" {
		return fmt.Errorf("artifact_url missing in version.json for %s", targetVersion)
	}

	zipPath := filepath.Join(r.wcfg.InstallDir, targetVersion+".zip")
	if err := r.github.DownloadArtifact(ctx, svcMeta.ArtifactURL, zipPath, r.wcfg.DownloadRetries); err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}
	defer func() {
		r.log.Debug("removing zip", "path", zipPath)
		os.Remove(zipPath)
	}()

	if err := r.deployer.Deploy(ctx, targetVersion, zipPath, previousVersion); err != nil {
		return err
	}

	if err := r.state.WriteVersion(targetVersion); err != nil {
		r.log.Warn("failed to write version", "error", err)
	}
	if err := r.state.SetHealthy(targetVersion); err != nil {
		r.log.Warn("failed to update state", "error", err)
	}

	r.log.Info("deploy complete", "version", targetVersion)
	return nil
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
	checkTrigger chan uint
	syncTrigger  chan struct{}

	mu       sync.Mutex
	watchers map[uint]watcherHandle
}

func NewAgent(db *gorm.DB, appCfg *config.AppConfig, log *Logger, checkTrigger chan uint, syncTrigger chan struct{}) *Agent {
	return &Agent{
		db:           db, 
		appCfg:       appCfg, 
		log:          log, 
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
	rw := NewRepoWatcher(dbWatcher, a.db, a.appCfg, a.log)

	log.Info("watcher starting",
		"service_name", dbWatcher.ServiceName,
		"metadata_url", dbWatcher.MetadataURL,
		"check_interval_sec", dbWatcher.CheckIntervalSec,
		"services", len(dbWatcher.Services),
	)

	// Run immediately on startup
	if err := rw.Run(ctx); err != nil && err != context.Canceled {
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
			if err := rw.Run(ctx); err != nil && err != context.Canceled {
				log.Error("check cycle failed", "error", err)
			}
		case <-trigger:
			log.Info("immediate check triggered via API")
			if err := rw.Run(ctx); err != nil && err != context.Canceled {
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