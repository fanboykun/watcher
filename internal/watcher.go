package internal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type Watcher struct {
	cfg      *Config
	log      *Logger
	github   *GitHubClient
	state    *StateManager
	deployer *Deployer
}

func NewWatcher(cfg *Config, log *Logger) *Watcher {
	return &Watcher{
		cfg:      cfg,
		log:      log,
		github:   NewGitHubClient(cfg.GitHubToken, log),
		state:    NewStateManager(cfg.InstallDir, log),
		deployer: NewDeployer(cfg, log),
	}
}

// Run performs one full check-and-deploy cycle.
// Called immediately on startup, then on every ticker tick.
func (w *Watcher) Run(ctx context.Context) error {
	w.log.Info("check cycle started", "environment", w.cfg.Environment)

	_ = w.state.SetChecked()

	// 1. Fetch remote version metadata
	meta, err := w.github.FetchMetadata(ctx, w.cfg.MetadataURL)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}

	svcMeta, ok := meta.Services[w.cfg.ServiceName]
	if !ok {
		return fmt.Errorf("service %q not found in version.json (available: %v)", w.cfg.ServiceName, keys(meta.Services))
	}

	targetVersion := svcMeta.Version
	w.log.Info("remote version", "target", targetVersion, "published_at", svcMeta.PublishedAt)

	// 2. Read local deployed version
	localVersion, err := w.state.ReadVersion()
	if err != nil {
		return fmt.Errorf("read local version: %w", err)
	}
	w.log.Info("local version", "current", localVersion)

	// 3. Nothing to do
	if localVersion == targetVersion {
		w.log.Info("already up to date")
		return nil
	}

	// 4. Deploy
	w.log.Info("version mismatch, deploying", "from", localVersion, "to", targetVersion)

	if err := w.deploy(ctx, svcMeta, targetVersion, localVersion); err != nil {
		_ = w.state.SetFailed(err.Error())
		return fmt.Errorf("deploy: %w", err)
	}

	return nil
}

func (w *Watcher) deploy(ctx context.Context, svcMeta ServiceMeta, targetVersion, previousVersion string) error {
	_ = w.state.SetDeploying(targetVersion)

	// Ensure install dir exists
	if err := os.MkdirAll(w.cfg.InstallDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	if svcMeta.ArtifactURL == "" {
		return fmt.Errorf("artifact_url missing in version.json for %s", targetVersion)
	}

	// Download artifact zip
	zipPath := filepath.Join(w.cfg.InstallDir, targetVersion+".zip")
	if err := w.github.DownloadArtifact(ctx, svcMeta.ArtifactURL, zipPath, w.cfg.DownloadRetries); err != nil {
		return fmt.Errorf("download artifact: %w", err)
	}

	// Always clean up the zip regardless of deploy outcome
	defer func() {
		w.log.Debug("removing zip", "path", zipPath)
		os.Remove(zipPath)
	}()

	// Extract, swap, restart, health check (with automatic rollback on failure)
	if err := w.deployer.Deploy(ctx, targetVersion, zipPath, previousVersion); err != nil {
		return err
	}

	// Persist new version on success
	if err := w.state.WriteVersion(targetVersion); err != nil {
		w.log.Warn("failed to write version.txt", "error", err)
	}
	if err := w.state.SetHealthy(targetVersion); err != nil {
		w.log.Warn("failed to update state", "error", err)
	}

	w.log.Info("deploy complete", "version", targetVersion)
	return nil
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}