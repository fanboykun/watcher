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

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

func (h *Handler) AgentLogs(c *gin.Context) {
	lines := 100
	if l := c.Query("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			lines = parsed
		}
	}

	logFile := filepath.Join(h.logDir, agent.LogFilename)

	content, err := tailFile(logFile, lines)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("agent log file not found: %s", logFile),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"log_file": logFile,
		"format":   "json",
		"lines":    content,
	})
}

// StreamAgentLogs streams the agent logs using Server-Sent Events (SSE).
func (h *Handler) StreamAgentLogs(c *gin.Context) {
	logFile := filepath.Join(h.logDir, agent.LogFilename)

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
