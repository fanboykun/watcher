package agent

import (
	"time"

	"github.com/fanboykun/watcher/internal/database"
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
}

func NewStateManager(db *gorm.DB, watcherID uint, log *Logger) *StateManager {
	return &StateManager{db: db, watcherID: watcherID, log: log}
}

func (s *StateManager) ReadVersion() (string, string, error) {
	var w database.Watcher
	if err := s.db.Select("current_version", "max_ignored_version").First(&w, s.watcherID).Error; err != nil {
		return "", "", nil // treat as no version yet
	}
	return w.CurrentVersion, w.MaxIgnoredVersion, nil
}

func (s *StateManager) WriteVersion(version string) error {
	return s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Updates(map[string]any{
			"current_version":     version,
			"max_ignored_version": "",
		}).Error
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

	// Record in deploy log
	dlog := database.DeployLog{
		WatcherID:   s.watcherID,
		Version:     version,
		FromVersion: fromVersion,
		Status:      string(StatusDeploying),
		StartedAt:   &now,
	}
	if err := s.db.Create(&dlog).Error; err != nil {
		return 0, err
	}
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
			"status":       string(StatusHealthy),
			"completed_at": &now,
			"duration_ms":  durationMs,
		})
	}
	return nil
}

func (s *StateManager) SetFailed(errMsg string) error {
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
			"status":       string(StatusFailed),
			"error":        errMsg,
			"completed_at": &now,
			"duration_ms":  durationMs,
		})
	}
	return nil
}

func (s *StateManager) SetRolledBack(version string) error {
	now := time.Now().UTC()
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":          string(StatusRollback),
			"current_version": version,
			"last_deployed":   &now,
		}).Error
	if err != nil {
		return err
	}

	return s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND completed_at IS NULL", s.watcherID).
		Updates(map[string]any{
			"status":       string(StatusRollback),
			"completed_at": &now,
		}).Error
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

// ConsecutiveFailuresForVersion returns the number of consecutive failed deploys
// for a specific version. Used to prevent infinite deploy retries.
func (s *StateManager) ConsecutiveFailuresForVersion(version string) int {
	var count int64
	s.db.Model(&database.DeployLog{}).
		Where("watcher_id = ? AND version = ? AND status = ?", s.watcherID, version, string(StatusFailed)).
		Count(&count)
	return int(count)
}