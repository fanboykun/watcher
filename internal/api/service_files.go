package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SyncServiceEnv(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	var req struct {
		EnvContent *string `json:"env_content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.db.Model(svc).Update("env_content", *req.EnvContent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	var watcher database.Watcher
	h.db.First(&watcher, svc.WatcherID)
	h.db.Preload("ConfigFiles").First(svc, svc.ID)
	if err := h.syncServiceFiles(svc, watcher.InstallDir); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.refreshActiveConfigSnapshot(svc.WatcherID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.db.Model(&database.Watcher{}).Where("id = ?", svc.WatcherID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	c.JSON(http.StatusOK, MessageResponse{Message: "Environment file updated and synced"})
}

func (h *Handler) syncServiceFiles(svc *database.Service, installDir string) error {
	if strings.TrimSpace(svc.EnvFile) != "" {
		if err := h.writeServiceFile(installDir, svc.EnvFile, svc.EnvContent); err != nil {
			return err
		}
	}
	for _, file := range svc.ConfigFiles {
		if strings.TrimSpace(file.FilePath) == "" || normalizeConfigFileTarget(file.Target) != "app_dir" {
			continue
		}
		if err := h.writeServiceFile(installDir, file.FilePath, file.Content); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) writeServiceFile(installDir, relativePath, content string) error {
	targetPath := filepath.Join(installDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("create service config directory %s: %w", targetPath, err)
	}
	if err := os.WriteFile(targetPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("write service config file %s: %w", targetPath, err)
	}
	return nil
}

// ── Deploy Log Detail ─────────────────────────────────────────

// GetDeployLog returns a single deploy log by ID (URL-able for GitHub Deployment API log_url).
