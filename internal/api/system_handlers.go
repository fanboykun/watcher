package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
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

// StreamAgentLogs streams the agent logs using Server-Sent Events (SSE).
func (h *Handler) StreamAgentLogs(c *gin.Context) {
	logType := c.DefaultQuery("type", "out")
	logFile := filepath.Join(h.logDir, "watcher."+logType+".log")

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	f, err := os.Open(logFile)
	if err != nil {
		c.SSEvent("error", "could not open log file")
		return
	}
	defer f.Close()

	// Send the last 50 lines to populate context
	content, _ := tailFile(logFile, 50)
	for _, line := range content {
		c.SSEvent("message", line)
	}
	c.Writer.Flush()

	f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						// Wait for next tick
						break
					}
					c.SSEvent("error", err.Error())
					return
				}
				c.SSEvent("message", line)
				c.Writer.Flush()
			}
		}
	}
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
