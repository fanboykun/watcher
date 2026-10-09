package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
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
	webhooks  *webhook.Service
}

// NewRepoWatcher creates a configured repo watcher.
func NewRepoWatcher(dbWatcher *database.Watcher, db *gorm.DB, appCfg *config.AppConfig, log *Logger, events *WatcherEventBus, webhookService *webhook.Service) *RepoWatcher {
	wcfg := WatcherConfigFromDB(dbWatcher)
	componentLog := log.WithWatcher(dbWatcher.ID, wcfg.Name)
	state := NewStateManager(db, dbWatcher.ID, componentLog, events, webhookService)
	return &RepoWatcher{
		wcfg:      wcfg,
		global:    appCfg,
		log:       componentLog,
		state:     state,
		deployer:  NewDeployer(wcfg, appCfg.NssmPath, componentLog, state.AppendDeployLog),
		db:        db,
		watcherID: dbWatcher.ID,
		webhooks:  webhookService,
	}
}

// Run performs one check-and-deploy cycle for this repo.
func (r *RepoWatcher) Run(ctx context.Context) error {
	if r.wcfg.Paused {
		return nil
	}
	trace := TraceFromContext(ctx)
	if trace.PollID == "" {
		trace = NewPollTrace(trace.RequestID, "scheduled")
	}
	ctx = WithTrace(ctx, trace)
	// Bind a copy for this run; shared runtime helpers never retain a previous trace.
	cycle := *r
	cycle.log = r.log.WithTrace(trace)
	state := *r.state
	state.log, state.trace = cycle.log, trace
	cycle.state = &state
	if r.deployer != nil {
		deployer := *r.deployer
		deployer.log, deployer.logFn = cycle.log, cycle.state.AppendDeployLog
		cycle.deployer = &deployer
	}
	started := time.Now()
	cycle.log.Info("poll started")
	err := cycle.runCycle(ctx)
	if errors.Is(err, context.Canceled) {
		cycle.log.Info("poll canceled", "duration_ms", time.Since(started).Milliseconds())
	} else if err != nil {
		cycle.log.Error("poll failed", "error", err, "duration_ms", time.Since(started).Milliseconds())
		if !cycle.state.pollErrorRecorded {
			cycle.state.RecordPollEvent("error", "", err.Error())
		}
	} else {
		cycle.log.Info("poll completed", "duration_ms", time.Since(started).Milliseconds())
	}
	if !errors.Is(err, context.Canceled) {
		status := cycle.state.pollStatus
		if status == "" {
			status = "completed"
		}
		now := time.Now().UTC()
		if dbErr := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"last_poll_status": status, "last_poll_error": cycle.state.pollError, "last_poll_id": trace.PollID, "last_poll_at": now}).Error; dbErr != nil {
			cycle.log.Warn("failed to save latest poll result", "error", dbErr)
		}
	}
	return err
}

func (r *RepoWatcher) runCycle(ctx context.Context) error {
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

	if IsVersionBlockedByRollback(targetVersion, maxIgnoredVersion) {
		r.log.Info("update skipped due to rollback high-watermark", "target", targetVersion, "ignored_upto", maxIgnoredVersion)
		r.state.RecordPollEvent("skipped", targetVersion, fmt.Sprintf("skipped (<= %s) because of manual rollback", maxIgnoredVersion))
		return nil
	}

	allowed, err := r.releaseApproved(targetVersion)
	if err != nil {
		return err
	}
	if !allowed {
		r.state.pollStatus = "intercepted"
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
			if r.recordDeployFailure(err, targetVersion, localVersion) {
				return nil
			}
		}
		return fmt.Errorf("deploy: %w", err)
	}

	return nil
}

// recordDeployFailure persists the target failure and any compensation result.
// It returns true only when the deployer confirmed a successful version rollback.

// releaseApproved reads live policy so approval and interception do not restart
// (or race with cancellation of) an in-flight deployment. Approval is version-bound.
func (r *RepoWatcher) releaseApproved(targetVersion string) (bool, error) {
	var watcher database.Watcher
	if err := r.db.Select("auto_deploy", "intercept_next_release", "pending_version", "approved_version", "status").First(&watcher, r.watcherID).Error; err != nil {
		return false, fmt.Errorf("read deployment policy: %w", err)
	}
	if watcher.ApprovedVersion == targetVersion {
		return true, nil
	}
	if watcher.AutoDeploy && !watcher.InterceptNextRelease && watcher.PendingVersion == "" && watcher.ApprovedVersion == "" {
		return true, nil
	}
	if watcher.PendingVersion != targetVersion || watcher.Status != "pending_approval" {
		result := r.db.Model(&database.Watcher{}).Where("id = ? AND approved_version = ? AND pending_version = ? AND status = ?", r.watcherID, watcher.ApprovedVersion, watcher.PendingVersion, watcher.Status).
			UpdateColumns(map[string]any{"status": "pending_approval", "pending_version": targetVersion, "approved_version": ""})
		if result.Error != nil {
			return false, fmt.Errorf("hold release for approval: %w", result.Error)
		}
		if result.RowsAffected > 0 {
			r.state.publish(EventStatusChanged, map[string]any{"status": "pending_approval", "pending_version": targetVersion})
			r.state.RecordPollEvent("intercepted", targetVersion, "held as candidate for manual approval")
		}
	}
	return false, nil
}
