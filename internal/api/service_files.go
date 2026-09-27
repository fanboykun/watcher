package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
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

// ── Configuration Revisions (Deployment Candidates) ──────────

func (h *Handler) ListServiceConfigRevisions(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	var revisions []database.ServiceConfigRevision
	if err := h.db.Where("service_id = ?", svc.ID).Order("id desc").Find(&revisions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": revisions})
}

func (h *Handler) UpdateServiceConfigRevision(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	targetVersion := c.Param("target")
	if targetVersion == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Target version is required"})
		return
	}

	var req struct {
		EnvContent string `json:"env_content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var revision database.ServiceConfigRevision
	err = h.db.Where("service_id = ? AND target_version = ?", svc.ID, targetVersion).First(&revision).Error
	if err != nil {
		// Create new
		revision = database.ServiceConfigRevision{
			ServiceID:     svc.ID,
			TargetVersion: targetVersion,
			EnvContent:    req.EnvContent,
		}
		if err := h.db.Create(&revision).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	} else {
		// Update existing
		if err := h.db.Model(&revision).Update("env_content", req.EnvContent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "Configuration candidate saved successfully"})
}

func (h *Handler) DeleteServiceConfigRevision(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	targetVersion := c.Param("target")
	if targetVersion == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Target version is required"})
		return
	}
	if err := h.db.Where("service_id = ? AND target_version = ?", svc.ID, targetVersion).Delete(&database.ServiceConfigRevision{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "Configuration candidate deleted successfully"})
}

// ── Configuration Snapshots (Read-Only History) ──────────────

func (h *Handler) GetServiceSnapshotEnv(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Version is required"})
		return
	}

	var watcher database.Watcher
	if err := h.db.Preload("Services").First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to load watcher"})
		return
	}

	serviceIndex := -1
	for i, s := range watcher.Services {
		if s.ID == svc.ID {
			serviceIndex = i
			break
		}
	}
	if serviceIndex == -1 {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Service not found in watcher"})
		return
	}

	wcfg := agent.WatcherConfigFromDB(&watcher)
	svcCfg := wcfg.Services[serviceIndex]

	snapshotRoot := agent.ConfigSnapshotPath(wcfg.InstallDir, version)

	safeName := svcCfg.WindowsServiceName
	if safeName == "" {
		safeName = svcCfg.IISSiteName
	}
	if safeName == "" {
		safeName = svcCfg.IISAppPool
	}

	var b strings.Builder
	for _, r := range safeName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%X", r)
		}
	}
	finalSafeName := b.String()
	if finalSafeName == "" || finalSafeName == "." || finalSafeName == ".." {
		finalSafeName = fmt.Sprintf("service-%d", serviceIndex+1)
	}

	envFilePath := filepath.Join(snapshotRoot, "services", finalSafeName, "env", svcCfg.EnvFile)

	fileContent, err := os.ReadFile(envFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment snapshot not found for this version"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"env_content": string(fileContent)})
}
