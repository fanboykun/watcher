package api

import (
	"net/http"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SystemStatus(c *gin.Context) {
	uptime := time.Since(h.startTime)

	var watcherCount, serviceCount int64
	h.db.Model(&database.Watcher{}).Count(&watcherCount)
	h.db.Model(&database.Service{}).Count(&serviceCount)

	// Count recent deploys (last 24h)
	var deployCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	h.db.Model(&database.DeployLog{}).Where("started_at > ?", dayAgo).Count(&deployCount)

	c.JSON(http.StatusOK, gin.H{
		"status":         "running",
		"version":        h.version,
		"uptime_seconds": int(uptime.Seconds()),
		"uptime_human":   formatDuration(uptime),
		"watcher_count":  watcherCount,
		"service_count":  serviceCount,
		"deploys_24h":    deployCount,
	})
}

// AgentLogs returns the last N lines of the watcher agent log file.
