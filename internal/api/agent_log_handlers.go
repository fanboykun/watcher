package api

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

func (h *Handler) AgentLogs(c *gin.Context) {
	source := c.DefaultQuery("source", "agent")
	if source != "agent" && source != "stdout" && source != "stderr" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "source must be agent, stdout, or stderr"})
		return
	}
	path, format, err := h.agentLogSource(c.Request.Context(), source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	serveLogFile(c, source, path, format, 0)
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
