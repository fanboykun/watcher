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

func (s *StateManager) ReadVersion() (string, error) {
	var watcher database.Watcher
	if err := s.db.Select("current_version").First(&watcher, s.watcherID).Error; err != nil {
		return "", nil // treat as no version yet
	}
	return watcher.CurrentVersion, nil
}

func (s *StateManager) WriteVersion(version string) error {
	return s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Update("current_version", version).Error
}

func (s *StateManager) SetChecked() error {
	now := time.Now().UTC()
	return s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		Update("last_checked", &now).Error
}

func (s *StateManager) SetDeploying(version, fromVersion string) error {
	now := time.Now().UTC()
	// Update watcher state
	err := s.db.Model(&database.Watcher{}).Where("id = ?", s.watcherID).
		UpdateColumns(map[string]any{
			"status":          string(StatusDeploying),
			"current_version": version,
			"last_checked":    &now,
			"last_error":      "",
		}).Error
	if err != nil {
		return err
	}

	// Record in deploy log
	return s.db.Create(&database.DeployLog{
		WatcherID:   s.watcherID,
		Version:     version,
		FromVersion: fromVersion,
		Status:      string(StatusDeploying),
		StartedAt:   &now,
	}).Error
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