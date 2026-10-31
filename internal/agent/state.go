package agent

import (
	"errors"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
	"gorm.io/gorm"
)

var errDeploymentPolicyChanged = errors.New("deployment policy changed before activation")

type DeployStatus string

const (
	StatusUnknown   DeployStatus = "unknown"
	StatusHealthy   DeployStatus = "healthy"
	StatusDeploying DeployStatus = "deploying"
	StatusFailed    DeployStatus = "failed"
	StatusRollback  DeployStatus = "rollback"
)

// StateManager manages deploy state in the database.
// Replaces the old file-based version.txt + state.json approach.
type StateManager struct {
	catalogID         uint
	deployLogID       uint
	recoverCatalog    bool
	db                *gorm.DB
	watcherID         uint
	log               *Logger
	events            *WatcherEventBus
	webhooks          *webhook.Service
	trace             Trace
	pollErrorRecorded bool
	pollStatus        string
	pollError         string
}

// NewStateManager creates persistence and event helpers scoped to one watcher.
func NewStateManager(db *gorm.DB, watcherID uint, log *Logger, events *WatcherEventBus, webhooks *webhook.Service) *StateManager {
	return &StateManager{db: db, watcherID: watcherID, log: log, events: events, webhooks: webhooks}
}

// ReadVersion reads version.
func (s *StateManager) ReadVersion() (string, string, error) {
	var w database.Watcher
	if err := s.db.Select("current_version", "max_ignored_version").First(&w, s.watcherID).Error; err != nil {
		return "", "", nil // treat as no version yet
	}
	return w.CurrentVersion, w.MaxIgnoredVersion, nil
}

// WriteVersion writes version.
func (s *StateManager) WriteVersion(version string) error {
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Updates(map[string]any{
			"current_version":     version,
			"max_ignored_version": "",
		}).Error
	if err == nil {
		s.publish(EventVersionChanged, map[string]any{"version": version})
	}
	return err
}

// SetChecked records that the watcher completed a metadata check.
func (s *StateManager) SetChecked() error {
	now := time.Now().UTC()
	return s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Update("last_checked", &now).Error
}

// SetDeploying opens or reuses a deployment attempt and marks the watcher deploying.
func (s *StateManager) SetDeploying(version, fromVersion string) (uint, error) {
	now := time.Now().UTC()
	var dlog database.DeployLog
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if s.catalogID != 0 {
			if s.deployLogID == 0 {
				return errDeploymentPolicyChanged
			}
			if err := tx.Where("id = ? AND watcher_id = ? AND kind = ? AND reason = ? AND version = ? AND status = ? AND completed_at IS NULL", s.deployLogID, s.watcherID, "deploy", "catalog_release", version, "in_progress").First(&dlog).Error; err != nil {
				return err
			}
		}
		query := tx.Model(&database.Watcher{}).Where("id = ? AND pending_catalog_id = ?", s.watcherID, s.catalogID)
		if s.catalogID != 0 {
			query = query.Where("status = ? AND approved_version = ? AND pending_version = ?", "approved", version, version)
		}
		result := query.UpdateColumns(map[string]any{"status": string(StatusDeploying), "last_checked": &now, "last_error": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errDeploymentPolicyChanged
		}
		if s.catalogID == 0 {
			err := tx.Where("watcher_id = ? AND completed_at IS NULL", s.watcherID).Order("id desc").First(&dlog).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if dlog.ID != 0 {
			return tx.Model(&dlog).Updates(map[string]any{"status": "in_progress", "version": version, "from_version": fromVersion, "started_at": &now, "error": "", "kind": "deploy"}).Error
		}
		dlog = database.DeployLog{WatcherID: s.watcherID, TriggeredBy: "agent", Kind: "deploy", Reason: "new_version", Version: version, FromVersion: fromVersion, Status: "in_progress", StartedAt: &now}
		if err := tx.Create(&dlog).Error; err != nil {
			return err
		}
		return tx.Model(&dlog).Update("root_attempt_id", dlog.ID).Error
	})
	if err != nil {
		return 0, err
	}
	s.deployLogID = dlog.ID
	s.publish(EventDeployStarted, map[string]any{
		"deploy_log_id": dlog.ID,
		"version":       version,
		"from_version":  fromVersion,
	})
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusDeploying)})
	return dlog.ID, nil
}

// SetGitHubDeploymentID stores the GitHub Deployment API ID on a deploy log record.
func (s *StateManager) SetGitHubDeploymentID(deployLogID uint, ghDeploymentID int64) error {
	return s.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).
		Update("github_deployment_id", ghDeploymentID).Error
}

// SetHealthy completes the active deploy attempt and records its version as healthy.
func (s *StateManager) SetHealthy(version string) error {
	now := time.Now().UTC()
	var attempt database.DeployLog
	err := s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("watcher_id = ? AND version = ? AND completed_at IS NULL", s.watcherID, version)
		if s.deployLogID != 0 {
			query = query.Where("id = ?", s.deployLogID)
		}
		err := query.First(&attempt).Error
		if err != nil && (!errors.Is(err, gorm.ErrRecordNotFound) || s.catalogID != 0) {
			return err
		}
		watcherQuery := tx.Model(&database.Watcher{}).Where("id = ?", s.watcherID)
		if s.catalogID != 0 {
			watcherQuery = watcherQuery.Where("pending_catalog_id = ? AND pending_version = ? AND approved_version = ? AND status = ?", s.catalogID, version, version, "deploying")
		}
		result := watcherQuery.UpdateColumns(map[string]any{"status": string(StatusHealthy), "last_deployed": &now, "last_error": "", "intercept_next_release": false, "pending_version": "", "pending_catalog_id": 0, "approved_version": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errDeploymentPolicyChanged
		}
		if attempt.ID == 0 {
			return nil
		}
		var durationMs int64
		if attempt.StartedAt != nil {
			durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
		}
		if err := tx.Model(&attempt).Updates(map[string]any{"status": "succeeded", "completed_at": &now, "duration_ms": durationMs}).Error; err != nil {
			return err
		}
		attempt.Status, attempt.CompletedAt, attempt.DurationMs = "succeeded", &now, durationMs
		return nil
	})
	if err != nil {
		return err
	}
	if attempt.ID != 0 {
		s.publish(EventDeployFinished, map[string]any{"deploy_log_id": attempt.ID, "status": "succeeded", "version": version})
		s.emitAttemptWebhook(&attempt)
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusHealthy)})
	return nil
}

// SetFailed records a deployment failure without a more specific phase.
func (s *StateManager) SetFailed(errMsg string) error {
	return s.SetFailedWithPhase(errMsg, "")
}

// SetFailedWithPhase fails the active deploy attempt and records its pipeline phase.
func (s *StateManager) SetFailedWithPhase(errMsg, phase string) error {
	now := time.Now().UTC()
	var attempt database.DeployLog
	changed := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("watcher_id = ? AND completed_at IS NULL", s.watcherID)
		if s.deployLogID != 0 {
			query = query.Where("id = ?", s.deployLogID)
		} else if s.catalogID != 0 {
			return errDeploymentPolicyChanged
		}
		err := query.First(&attempt).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && s.deployLogID != 0 {
			return nil // This exact attempt already finished; a retry may now own approval.
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		watcherQuery := tx.Model(&database.Watcher{}).Where("id = ?", s.watcherID)
		updates := map[string]any{"status": string(StatusFailed), "last_error": errMsg}
		if s.catalogID != 0 {
			if attempt.Kind != "deploy" || attempt.Reason != "catalog_release" {
				return errDeploymentPolicyChanged
			}
			watcherQuery = watcherQuery.Where("pending_catalog_id = ? AND pending_version = ?", s.catalogID, attempt.Version)
			if s.recoverCatalog {
				watcherQuery = watcherQuery.Where("approved_version IN ? AND status IN ?", []string{attempt.Version, ""}, []string{"approved", "deploying", "failed", "healthy", "pending_approval"})
			} else {
				watcherQuery = watcherQuery.Where("approved_version = ? AND status IN ?", attempt.Version, []string{"approved", "deploying"})
			}
			updates["approved_version"] = ""
		}
		result := watcherQuery.UpdateColumns(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if attempt.ID != 0 {
			var durationMs int64
			if attempt.StartedAt != nil {
				durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
			}
			result := tx.Model(&attempt).Where("completed_at IS NULL").Updates(map[string]any{"status": "failed", "error": errMsg, "failure_phase": phase, "completed_at": &now, "duration_ms": durationMs})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errDeploymentPolicyChanged
			}
			attempt.Status, attempt.Error, attempt.FailurePhase = "failed", errMsg, phase
			attempt.CompletedAt, attempt.DurationMs = &now, durationMs
		}
		changed = true
		return nil
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if attempt.ID != 0 {
		s.publish(EventDeployFinished, map[string]any{"deploy_log_id": attempt.ID, "status": "failed", "error": errMsg})
		s.emitAttemptWebhook(&attempt)
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusFailed), "error": errMsg})
	return nil
}

// SetDeferred closes the active attempt as "deferred" so it is not counted by
// ConsecutiveFailuresForVersion, and restores the watcher state SetDeploying replaced.
func (s *StateManager) SetDeferred(errMsg, previousStatus, previousError string) error {
	if previousStatus == "" || previousStatus == string(StatusDeploying) {
		previousStatus = string(StatusUnknown)
	}
	now := time.Now().UTC()
	var attempt database.DeployLog
	err := s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("watcher_id = ? AND completed_at IS NULL", s.watcherID)
		if s.deployLogID != 0 {
			query = query.Where("id = ?", s.deployLogID)
		}
		if err := query.First(&attempt).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Model(&database.Watcher{}).Where("id = ? AND status = ?", s.watcherID, string(StatusDeploying)).
			UpdateColumns(map[string]any{"status": previousStatus, "last_error": previousError}).Error; err != nil {
			return err
		}
		if attempt.ID == 0 {
			return nil
		}
		var durationMs int64
		if attempt.StartedAt != nil {
			durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
		}
		return tx.Model(&attempt).Updates(map[string]any{"status": "deferred", "error": errMsg, "completed_at": &now, "duration_ms": durationMs}).Error
	})
	if err != nil {
		return err
	}
	if attempt.ID != 0 {
		s.publish(EventDeployFinished, map[string]any{"deploy_log_id": attempt.ID, "status": "deferred", "error": errMsg})
	}
	s.publish(EventStatusChanged, map[string]any{"status": previousStatus})
	return nil
}

// SetRolledBack records a successful rollback and advances the rollback high-watermark.
func (s *StateManager) SetRolledBack(version string) error {
	now := time.Now().UTC()
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":          string(StatusHealthy),
			"current_version": version,
			"last_deployed":   &now,
			"last_error":      "",
		}).Error
	if err != nil {
		return err
	}

	err = s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND kind = ? AND completed_at IS NULL", s.watcherID, "rollback").
		Updates(map[string]any{
			"status":       "succeeded",
			"completed_at": &now,
		}).Error
	if err == nil {
		var log database.DeployLog
		if err := s.db.Where("watcher_id = ? AND kind = ? AND completed_at = ?", s.watcherID, "rollback", &now).Order("id desc").First(&log).Error; err == nil {
			log.Status = "succeeded"
			log.CompletedAt = &now
			s.emitAttemptWebhook(&log)
		}
		s.publish(EventStatusChanged, map[string]any{"status": string(StatusHealthy), "version": version})
		s.publish(EventVersionChanged, map[string]any{"version": version})
	}
	return err
}

// AppendDeployLog appends deploy log.
func (s *StateManager) AppendDeployLog(text string) {
	query := s.db.Model(&database.DeployLog{}).Where("watcher_id = ? AND completed_at IS NULL", s.watcherID)
	if s.deployLogID != 0 {
		query = query.Where("id = ?", s.deployLogID)
	}
	err := query.UpdateColumn("logs", gorm.Expr("COALESCE(logs, '') || ?", text+"\n")).Error
	if err != nil {
		s.log.Warn("failed to append deploy log", "error", err)
	}
}

// RecordPollEvent appends a poll result and trims the watcher history to its limit.
func (s *StateManager) RecordPollEvent(status, remoteVersion, errMsg string) {
	s.pollStatus, s.pollError = status, errMsg
	if status == "error" {
		s.pollErrorRecorded = true
	}
	evt := database.PollEvent{
		PollID: s.trace.PollID, RequestID: s.trace.RequestID, CorrelationID: s.trace.CorrelationID, TriggeredBy: s.trace.TriggeredBy,
		WatcherID:     s.watcherID,
		Status:        status,
		RemoteVersion: remoteVersion,
		Error:         errMsg,
	}
	if err := s.db.Create(&evt).Error; err != nil {
		s.log.Warn("failed to record poll event", "error", err)
	}
	s.publish(EventPollEvent, map[string]any{
		"poll_id": s.trace.PollID, "request_id": s.trace.RequestID, "correlation_id": s.trace.CorrelationID, "triggered_by": s.trace.TriggeredBy,
		"status":         status,
		"remote_version": remoteVersion,
		"error":          errMsg,
	})

	// Keep only the last 50 poll events for this watcher
	s.db.Exec(`
		DELETE FROM poll_events 
		WHERE watcher_id = ? 
		AND id NOT IN (
			SELECT id FROM poll_events 
			WHERE watcher_id = ? 
			ORDER BY id DESC 
			LIMIT 50
		)`, s.watcherID, s.watcherID)
}

// publish broadcasts a watcher event and forwards it to configured webhooks.
func (s *StateManager) publish(eventType string, data map[string]any) {
	if s.events == nil {
		return
	}
	s.events.Publish(s.watcherID, WatcherEvent{
		Type: eventType,
		Data: data,
	})
}

// ConsecutiveFailuresForVersion returns the number of consecutive failed deploys
// for a specific version. Used to prevent infinite deploy retries.
func (s *StateManager) ConsecutiveFailuresForVersion(version string) int {
	var count int64
	s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND version = ? AND kind = ? AND status = ?", s.watcherID, version, "deploy", "failed").
		Count(&count)
	return int(count)
}

// HasPendingManualDeploy reports whether an open manual attempt may bypass retry suspension.
func (s *StateManager) HasPendingManualDeploy() bool {
	var count int64
	s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND kind = ? AND completed_at IS NULL AND triggered_by = ?", s.watcherID, "deploy", "manual").
		Count(&count)
	return count > 0
}

// StartRollbackAttempt starts rollback attempt.
func (s *StateManager) StartRollbackAttempt(version, fromVersion, failedTargetVersion, reason, triggeredBy string, parentAttemptID, rootAttemptID *uint) (uint, error) {
	now := time.Now().UTC()
	attempt := database.DeployLog{
		WatcherID:           s.watcherID,
		TriggeredBy:         triggeredBy,
		Kind:                "rollback",
		Reason:              reason,
		Version:             version,
		FromVersion:         fromVersion,
		FailedTargetVersion: failedTargetVersion,
		Status:              "in_progress",
		ParentAttemptID:     parentAttemptID,
		RootAttemptID:       rootAttemptID,
		StartedAt:           &now,
	}
	if err := s.db.Create(&attempt).Error; err != nil {
		return 0, err
	}
	if attempt.RootAttemptID == nil {
		_ = s.db.Model(&attempt).Update("root_attempt_id", attempt.ID).Error
	}
	return attempt.ID, nil
}

// CompleteRollbackAttempt completes rollback attempt.
func (s *StateManager) CompleteRollbackAttempt(attemptID uint, version string, maxIgnoredVersion string) error {
	return s.finishRollbackAttempt(attemptID, version, maxIgnoredVersion, "")
}

// FailRollbackAttempt marks rollback attempt as failed.
func (s *StateManager) FailRollbackAttempt(attemptID uint, errMsg string) error {
	return s.finishRollbackAttempt(attemptID, "", "", errMsg)
}

// The open rollback log remains an admission guard until its watcher state commits.
func (s *StateManager) finishRollbackAttempt(attemptID uint, version, maxIgnoredVersion, errMsg string) error {
	now := time.Now().UTC()
	var attempt database.DeployLog
	status, watcherStatus := "succeeded", string(StatusHealthy)
	if errMsg != "" {
		status, watcherStatus = "failed", string(StatusFailed)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND watcher_id = ? AND kind = ? AND completed_at IS NULL", attemptID, s.watcherID, "rollback").First(&attempt).Error; err != nil {
			return err
		}
		var durationMs int64
		if attempt.StartedAt != nil {
			durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
		}
		if err := tx.Model(&attempt).Updates(map[string]any{"status": status, "error": errMsg, "completed_at": &now, "duration_ms": durationMs}).Error; err != nil {
			return err
		}
		updates := map[string]any{"status": watcherStatus, "last_error": errMsg}
		if errMsg == "" {
			updates["current_version"], updates["max_ignored_version"], updates["last_deployed"] = version, maxIgnoredVersion, &now
		}
		if err := tx.Model(&database.Watcher{}).Where("id = ?", s.watcherID).Updates(updates).Error; err != nil {
			return err
		}
		attempt.Status, attempt.Error, attempt.CompletedAt, attempt.DurationMs = status, errMsg, &now, durationMs
		return nil
	})
	if err != nil {
		return err
	}
	data := map[string]any{"status": watcherStatus}
	if errMsg != "" {
		data["error"] = errMsg
	} else {
		data["version"] = version
	}
	s.publish(EventStatusChanged, data)
	if errMsg == "" {
		s.publish(EventVersionChanged, map[string]any{"version": version})
	}
	s.emitAttemptWebhook(&attempt)
	return nil
}

// emitAttemptWebhook emits the webhook event corresponding to a completed deployment attempt.
func (s *StateManager) emitAttemptWebhook(log *database.DeployLog) {
	if s.webhooks == nil || log == nil {
		return
	}
	var watcher database.Watcher
	if err := s.db.First(&watcher, s.watcherID).Error; err != nil {
		s.log.Error("failed to load watcher for attempt webhook", "deploy_log_id", log.ID, "error", err)
		return
	}
	if err := s.webhooks.EmitAttemptEventTx(s.db, &watcher, log); err != nil {
		s.log.Error("failed to persist attempt webhook", "deploy_log_id", log.ID, "error", err)
	}
}
