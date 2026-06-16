package agent

import (
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
	"gorm.io/gorm"
)

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
	db        *gorm.DB
	watcherID uint
	log       *Logger
	events    *WatcherEventBus
	webhooks  *webhook.Service
}

func NewStateManager(db *gorm.DB, watcherID uint, log *Logger, events *WatcherEventBus, webhooks *webhook.Service) *StateManager {
	return &StateManager{db: db, watcherID: watcherID, log: log, events: events, webhooks: webhooks}
}

func (s *StateManager) ReadVersion() (string, string, error) {
	var w database.Watcher
	if err := s.db.Select("current_version", "max_ignored_version").First(&w, s.watcherID).Error; err != nil {
		return "", "", nil // treat as no version yet
	}
	return w.CurrentVersion, w.MaxIgnoredVersion, nil
}

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

func (s *StateManager) SetChecked() error {
	now := time.Now().UTC()
	return s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Update("last_checked", &now).Error
}

func (s *StateManager) SetDeploying(version, fromVersion string) (uint, error) {
	now := time.Now().UTC()
	// Update watcher state
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":       string(StatusDeploying),
			"last_checked": &now,
			"last_error":   "",
		}).Error
	if err != nil {
		return 0, err
	}

	// Reuse an existing open deploy log if present (e.g. manual redeploy queued from API),
	// otherwise create a new deploy log.
	var dlog database.DeployLog
	if err := s.db.Where("watcher_id = ? AND completed_at IS NULL", s.watcherID).Order("id desc").First(&dlog).Error; err == nil {
		if err := s.db.Model(&dlog).Updates(map[string]any{
			"status":       "in_progress",
			"version":      version,
			"from_version": fromVersion,
			"started_at":   &now,
			"error":        "",
			"kind":         "deploy",
			"reason":       "manual_redeploy",
		}).Error; err != nil {
			return 0, err
		}
	} else {
		dlog = database.DeployLog{
			WatcherID:   s.watcherID,
			TriggeredBy: "agent",
			Kind:        "deploy",
			Reason:      "new_version",
			Version:     version,
			FromVersion: fromVersion,
			Status:      "in_progress",
			StartedAt:   &now,
		}
		if err := s.db.Create(&dlog).Error; err != nil {
			return 0, err
		}
		_ = s.db.Model(&dlog).Update("root_attempt_id", dlog.ID).Error
		dlog.RootAttemptID = &dlog.ID
	}
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

func (s *StateManager) SetHealthy(version string) error {
	now := time.Now().UTC()
	// Update watcher state
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":        string(StatusHealthy),
			"last_deployed": &now,
			"last_error":    "",
		}).Error
	if err != nil {
		return err
	}

	// Update the latest deploy log — set completed + compute duration
	var log database.DeployLog
	if err := s.db.Where("watcher_id = ? AND version = ? AND completed_at IS NULL", s.watcherID, version).
		First(&log).Error; err == nil {
		var durationMs int64
		if log.StartedAt != nil {
			durationMs = now.Sub(*log.StartedAt).Milliseconds()
		}
		s.db.Model(&log).Updates(map[string]any{
			"status":       "succeeded",
			"completed_at": &now,
			"duration_ms":  durationMs,
		})
		log.Status = "succeeded"
		log.CompletedAt = &now
		log.DurationMs = durationMs
		s.publish(EventDeployFinished, map[string]any{
			"deploy_log_id": log.ID,
			"status":        "succeeded",
			"version":       version,
		})
		s.emitAttemptWebhook(&log)
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusHealthy)})
	return nil
}

func (s *StateManager) SetFailed(errMsg string) error {
	return s.SetFailedWithPhase(errMsg, "")
}

func (s *StateManager) SetFailedWithPhase(errMsg, phase string) error {
	now := time.Now().UTC()
	// Update watcher state
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":     string(StatusFailed),
			"last_error": errMsg,
		}).Error
	if err != nil {
		return err
	}

	// Update the latest deploy log — compute duration
	var log database.DeployLog
	if err := s.db.Where("watcher_id = ? AND completed_at IS NULL", s.watcherID).
		First(&log).Error; err == nil {
		var durationMs int64
		if log.StartedAt != nil {
			durationMs = now.Sub(*log.StartedAt).Milliseconds()
		}
		s.db.Model(&log).Updates(map[string]any{
			"status":        "failed",
			"error":         errMsg,
			"failure_phase": phase,
			"completed_at":  &now,
			"duration_ms":   durationMs,
		})
		log.Status = "failed"
		log.Error = errMsg
		log.FailurePhase = phase
		log.CompletedAt = &now
		log.DurationMs = durationMs
		s.publish(EventDeployFinished, map[string]any{
			"deploy_log_id": log.ID,
			"status":        "failed",
			"error":         errMsg,
		})
		s.emitAttemptWebhook(&log)
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusFailed), "error": errMsg})
	return nil
}

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

func (s *StateManager) AppendDeployLog(text string) {
	err := s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND completed_at IS NULL", s.watcherID).
		UpdateColumn("logs", gorm.Expr("COALESCE(logs, '') || ?", text+"\n")).Error
	if err != nil {
		s.log.Warn("failed to append deploy log", "error", err)
	}
}

func (s *StateManager) RecordPollEvent(status, remoteVersion, errMsg string) {
	evt := database.PollEvent{
		WatcherID:     s.watcherID,
		Status:        status,
		RemoteVersion: remoteVersion,
		Error:         errMsg,
	}
	if err := s.db.Create(&evt).Error; err != nil {
		s.log.Warn("failed to record poll event", "error", err)
	}
	s.publish(EventPollEvent, map[string]any{
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

func (s *StateManager) HasPendingManualDeploy() bool {
	var count int64
	s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND kind = ? AND completed_at IS NULL AND triggered_by = ?", s.watcherID, "deploy", "manual").
		Count(&count)
	return count > 0
}

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

func (s *StateManager) CompleteRollbackAttempt(attemptID uint, version string, maxIgnoredVersion string) error {
	now := time.Now().UTC()
	var attempt database.DeployLog
	if err := s.db.First(&attempt, attemptID).Error; err != nil {
		return err
	}
	durationMs := int64(0)
	if attempt.StartedAt != nil {
		durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
	}
	if err := s.db.Model(&attempt).Updates(map[string]any{
		"status":       "succeeded",
		"completed_at": &now,
		"duration_ms":  durationMs,
	}).Error; err != nil {
		return err
	}
	attempt.Status = "succeeded"
	attempt.CompletedAt = &now
	attempt.DurationMs = durationMs

	if err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).Updates(map[string]any{
		"status":              string(StatusHealthy),
		"current_version":     version,
		"max_ignored_version": maxIgnoredVersion,
		"last_deployed":       &now,
		"last_error":          "",
	}).Error; err != nil {
		return err
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusHealthy), "version": version})
	s.publish(EventVersionChanged, map[string]any{"version": version})
	s.emitAttemptWebhook(&attempt)
	return nil
}

func (s *StateManager) FailRollbackAttempt(attemptID uint, errMsg string) error {
	now := time.Now().UTC()
	var attempt database.DeployLog
	if err := s.db.First(&attempt, attemptID).Error; err != nil {
		return err
	}
	durationMs := int64(0)
	if attempt.StartedAt != nil {
		durationMs = now.Sub(*attempt.StartedAt).Milliseconds()
	}
	if err := s.db.Model(&attempt).Updates(map[string]any{
		"status":       "failed",
		"error":        errMsg,
		"completed_at": &now,
		"duration_ms":  durationMs,
	}).Error; err != nil {
		return err
	}
	attempt.Status = "failed"
	attempt.Error = errMsg
	attempt.CompletedAt = &now
	attempt.DurationMs = durationMs
	if err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).Updates(map[string]any{
		"status":     string(StatusFailed),
		"last_error": errMsg,
	}).Error; err != nil {
		return err
	}
	s.publish(EventStatusChanged, map[string]any{"status": string(StatusFailed), "error": errMsg})
	s.emitAttemptWebhook(&attempt)
	return nil
}

func (s *StateManager) emitAttemptWebhook(log *database.DeployLog) {
	if s.webhooks == nil || log == nil {
		return
	}
	var watcher database.Watcher
	if err := s.db.First(&watcher, s.watcherID).Error; err != nil {
		return
	}
	_ = s.webhooks.EmitAttemptEventTx(s.db, &watcher, log)
}
