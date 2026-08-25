package api

import (
	"fmt"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

// refreshActiveConfigSnapshot keeps the active version's trusted snapshot in sync.
func (h *Handler) refreshActiveConfigSnapshot(watcherID uint) error {
	var watcher database.Watcher
	if err := h.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, watcherID).Error; err != nil {
		return fmt.Errorf("load watcher for config snapshot: %w", err)
	}
	version := strings.TrimSpace(watcher.CurrentVersion)
	if version == "" {
		return nil
	}
	if err := agent.CaptureConfigSnapshot(agent.WatcherConfigFromDB(&watcher), version, agent.SnapshotSourceActiveUpdate); err != nil {
		return fmt.Errorf("refresh config snapshot for active version %s: %w", version, err)
	}
	return nil
}
