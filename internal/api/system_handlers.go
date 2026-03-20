package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

// SystemStatus returns agent health, uptime, version, and summary counts.
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
		"status":           "running",
		"version":          h.version,
		"uptime_seconds":   int(uptime.Seconds()),
		"uptime_human":     formatDuration(uptime),
		"watcher_count":    watcherCount,
		"service_count":    serviceCount,
		"deploys_24h":      deployCount,
	})
}

// AgentLogs returns the last N lines of the watcher agent log file.
func (h *Handler) AgentLogs(c *gin.Context) {
	lines := 100
	if l := c.Query("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			lines = parsed
		}
	}

	logType := c.DefaultQuery("type", "out") // "out" or "err"
	logFile := filepath.Join(h.logDir, "watcher."+logType+".log")

	content, err := tailFile(logFile, lines)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("agent log file not found: %s", logFile),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"log_file": logFile,
		"type":     logType,
		"lines":    content,
	})
}

// TriggerCheck sends a watcher ID to the check trigger channel
// so the agent runs an immediate poll cycle.
func (h *Handler) TriggerCheck(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if h.checkTrigger == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "check trigger not available"})
		return
	}

	// Non-blocking send — if buffer is full, the check is already pending
	select {
	case h.checkTrigger <- watcher.ID:
		c.JSON(http.StatusAccepted, MessageResponse{
			Message: fmt.Sprintf("immediate check triggered for watcher %q", watcher.Name),
		})
	default:
		c.JSON(http.StatusAccepted, MessageResponse{
			Message: fmt.Sprintf("check already pending for watcher %q", watcher.Name),
		})
	}
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
