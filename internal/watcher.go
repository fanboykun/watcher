package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// RepoWatcher manages the poll loop for a single WatcherConfig entry.
// Multiple RepoWatchers run concurrently inside the main agent.
type RepoWatcher struct {
	cfg      *WatcherConfig
	global   *Config
	log      *Logger
	github   *GitHubClient
	state    *StateManager
	deployer *Deployer
}

func NewRepoWatcher(wcfg *WatcherConfig, global *Config, log *Logger) *RepoWatcher {
	return &RepoWatcher{
		cfg:      wcfg,
		global:   global,
		log:      log.WithComponent(wcfg.Name),
		github:   NewGitHubClient(global.GitHubToken, log.WithComponent(wcfg.Name)),
		state:    NewStateManager(wcfg.InstallDir, log.WithComponent(wcfg.Name)),
		deployer: NewDeployer(wcfg, global.NssmPath, log.WithComponent(wcfg.Name)),
	}
}

// Run performs one check-and-deploy cycle for this repo.
func (r *RepoWatcher) Run(ctx context.Context) error {
	r.log.Info("check cycle", "metadata_url", r.cfg.MetadataURL)

	_ = r.state.SetChecked()

	meta, err := r.github.FetchMetadata(ctx, r.cfg.MetadataURL)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}

	svcMeta, ok := meta.Services[r.cfg.ServiceName]
	if !ok {
		return fmt.Errorf("service %q not in version.json (available: %v)", r.cfg.ServiceName, keys(meta.Services))
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
	_ = r.state.SetDeploying(targetVersion)

	if err := os.MkdirAll(r.cfg.InstallDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	if svcMeta.ArtifactURL == "" {
		return fmt.Errorf("artifact_url missing in version.json for %s", targetVersion)
	}

	zipPath := filepath.Join(r.cfg.InstallDir, targetVersion+".zip")
	if err := r.github.DownloadArtifact(ctx, svcMeta.ArtifactURL, zipPath, r.cfg.DownloadRetries); err != nil {
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
		r.log.Warn("failed to write version.txt", "error", err)
	}
	if err := r.state.SetHealthy(targetVersion); err != nil {
		r.log.Warn("failed to update state", "error", err)
	}

	r.log.Info("deploy complete", "version", targetVersion)
	return nil
}

// ── Agent — runs all RepoWatchers concurrently ────────────────────────────────

// Agent manages all RepoWatcher instances, one goroutine per watcher entry
type Agent struct {
	cfg *Config
	log *Logger
}

func NewAgent(cfg *Config, log *Logger) *Agent {
	return &Agent{cfg: cfg, log: log}
}

// Run starts all repo watchers concurrently and blocks until ctx is cancelled.
// Each watcher runs on its own ticker with its own check_interval_sec.
func (a *Agent) Run(ctx context.Context) {
	var wg sync.WaitGroup

	for i := range a.cfg.Watchers {
		wcfg := &a.cfg.Watchers[i]
		wg.Add(1)

		go func(wcfg *WatcherConfig) {
			defer wg.Done()
			a.runWatcher(ctx, wcfg)
		}(wcfg)
	}

	wg.Wait()
	a.log.Info("all watchers stopped")
}

func (a *Agent) runWatcher(ctx context.Context, wcfg *WatcherConfig) {
	log := a.log.WithComponent(wcfg.Name)
	rw := NewRepoWatcher(wcfg, a.cfg, a.log)

	log.Info("watcher starting",
		"service_name", wcfg.ServiceName,
		"metadata_url", wcfg.MetadataURL,
		"check_interval_sec", wcfg.CheckIntervalSec,
		"services", len(wcfg.Services),
	)

	// Run immediately on startup
	if err := rw.Run(ctx); err != nil && err != context.Canceled {
		log.Error("initial check failed", "error", err)
	}

	ticker := newTicker(wcfg.CheckIntervalSec)
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