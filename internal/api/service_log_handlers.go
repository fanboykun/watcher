package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetServiceLogs(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	// Find the watcher's install_dir to locate log files
	var watcher database.Watcher
	if err := h.db.Select("install_dir").First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "parent watcher not found"})
		return
	}

	logType := c.DefaultQuery("type", "out") // "out" or "err"
	logFile := filepath.Join(watcher.InstallDir, "logs", svc.WindowsServiceName+"."+logType+".log")

	lines := 100
	if l := c.Query("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			lines = parsed
		}
	}

	content, err := tailFile(logFile, lines)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("log file not found: %s", logFile)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service":  svc.WindowsServiceName,
		"log_file": logFile,
		"type":     logType,
		"lines":    content,
	})
}

// ── Service deploy history ────────────────────────────────────────────
