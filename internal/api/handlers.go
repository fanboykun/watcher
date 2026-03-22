package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// defaultServiceType returns "nssm" if the provided type is empty.
func defaultServiceType(t string) string {
	if t == "" {
		return "nssm"
	}
	return t
}

// Handler holds dependencies for all API endpoints.
type Handler struct {
	db           *gorm.DB
	nssmPath     string
	logDir       string
	version      string
	githubToken  string
	startTime    time.Time
	checkTrigger chan uint    // send watcher ID for immediate poll
	syncTrigger  chan struct{} // trigger background agent to sync DB
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *gorm.DB, nssmPath, logDir, version, githubToken string, checkTrigger chan uint, syncTrigger chan struct{}) *Handler {
	return &Handler{
		db:           db,
		nssmPath:     nssmPath,
		logDir:       logDir,
		version:      version,
		githubToken:  githubToken,
		startTime:    time.Now(),
		checkTrigger: checkTrigger,
		syncTrigger:  syncTrigger,
	}
}

// ── Watcher CRUD ──────────────────────────────────────────────

// ListWatchers returns all watchers with their services and current state.
func (h *Handler) ListWatchers(c *gin.Context) {
	var watchers []database.Watcher
	if err := h.db.Preload("Services").Find(&watchers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, watchers)
}

// GetWatcher returns a single watcher with services.
func (h *Handler) GetWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return // response already sent
	}
	c.JSON(http.StatusOK, watcher)
}

// CreateWatcher creates a new watcher entry with optional inline services.
func (h *Handler) CreateWatcher(c *gin.Context) {
	var req CreateWatcherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	watcher := database.Watcher{
		Name:             req.Name,
		ServiceName:      req.ServiceName,
		MetadataURL:      req.MetadataURL,
		CheckIntervalSec: withDefault(req.CheckIntervalSec, 60),
		DownloadRetries:  withDefault(req.DownloadRetries, 3),
		InstallDir:       req.InstallDir,
		HcEnabled:        req.HcEnabled,
		HcURL:            req.HcURL,
		HcRetries:        withDefault(req.HcRetries, 10),
		HcIntervalSec:    withDefault(req.HcIntervalSec, 3),
		HcTimeoutSec:     withDefault(req.HcTimeoutSec, 5),
		Paused:           req.Paused,
		Status:           "unknown",
	}

	// Create watcher
	if err := h.db.Create(&watcher).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	// Create inline services if provided
	for _, svcReq := range req.Services {
		svc := database.Service{
			WatcherID:          watcher.ID,
			ServiceType:        defaultServiceType(svcReq.ServiceType),
			WindowsServiceName: svcReq.WindowsServiceName,
			BinaryName:         svcReq.BinaryName,
			EnvFile:            svcReq.EnvFile,
			HealthCheckURL:     svcReq.HealthCheckURL,
			IISAppPool:         svcReq.IISAppPool,
			IISSiteName:        svcReq.IISSiteName,
		}
		if err := h.db.Create(&svc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	h.triggerSync()

	// Reload with services
	h.db.Preload("Services").First(&watcher, watcher.ID)
	c.JSON(http.StatusCreated, watcher)
}

// UpdateWatcher updates watcher fields (partial update via pointer fields).
func (h *Handler) UpdateWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var req UpdateWatcherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.ServiceName != nil {
		updates["service_name"] = *req.ServiceName
	}
	if req.MetadataURL != nil {
		updates["metadata_url"] = *req.MetadataURL
	}
	if req.CheckIntervalSec != nil {
		updates["check_interval_sec"] = *req.CheckIntervalSec
	}
	if req.DownloadRetries != nil {
		updates["download_retries"] = *req.DownloadRetries
	}
	if req.InstallDir != nil {
		updates["install_dir"] = *req.InstallDir
	}
	if req.HcEnabled != nil {
		updates["hc_enabled"] = *req.HcEnabled
	}
	if req.HcURL != nil {
		updates["hc_url"] = *req.HcURL
	}
	if req.HcRetries != nil {
		updates["hc_retries"] = *req.HcRetries
	}
	if req.HcIntervalSec != nil {
		updates["hc_interval_sec"] = *req.HcIntervalSec
	}
	if req.HcTimeoutSec != nil {
		updates["hc_timeout_sec"] = *req.HcTimeoutSec
	}
	if req.Paused != nil {
		updates["paused"] = *req.Paused
	}

	if len(updates) > 0 {
		if err := h.db.Model(watcher).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	h.triggerSync()

	// Reload
	h.db.Preload("Services").First(watcher, watcher.ID)
	c.JSON(http.StatusOK, watcher)
}

// DeleteWatcher soft-deletes a watcher and its services (cascade).
func (h *Handler) DeleteWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	// Delete services first (soft delete)
	h.db.Where("watcher_id = ?", watcher.ID).Delete(&database.Service{})

	if err := h.db.Delete(watcher).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.triggerSync()
	c.JSON(http.StatusOK, MessageResponse{Message: "watcher deleted"})
}
// ── Service CRUD (nested under watcher) ───────────────────────

// GetServiceDetail returns a single service with parent watcher info (flat route).
func (h *Handler) GetServiceDetail(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	var watcher database.Watcher
	h.db.Select("id", "name", "service_name", "install_dir", "current_version", "status").
		First(&watcher, svc.WatcherID)

	c.JSON(http.StatusOK, gin.H{
		"service": svc,
		"watcher": watcher,
	})
}

// ── Service CRUD (nested under watcher) ──────────────────────

// ListServices returns all services for a watcher.
func (h *Handler) ListServices(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var services []database.Service
	if err := h.db.Where("watcher_id = ?", watcher.ID).Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, services)
}

// CreateService adds a service to a watcher.
func (h *Handler) CreateService(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var req CreateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	svc := database.Service{
		WatcherID:          watcher.ID,
		ServiceType:        defaultServiceType(req.ServiceType),
		WindowsServiceName: req.WindowsServiceName,
		BinaryName:         req.BinaryName,
		EnvFile:            req.EnvFile,
		HealthCheckURL:     req.HealthCheckURL,
		IISAppPool:         req.IISAppPool,
		IISSiteName:        req.IISSiteName,
		PublicURL:          req.PublicURL,
		EnvContent:         req.EnvContent,
	}

	if err := h.db.Create(&svc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.syncServiceEnvFile(&svc, watcher.InstallDir)
	
	h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	c.JSON(http.StatusCreated, svc)
}

// UpdateService updates a service (partial update).
func (h *Handler) UpdateService(c *gin.Context) {
	svc, err := h.findService(c)
	if err != nil {
		return
	}

	var req UpdateServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	updates := map[string]any{}
	if req.ServiceType != nil {
		updates["service_type"] = *req.ServiceType
	}
	if req.WindowsServiceName != nil {
		updates["windows_service_name"] = *req.WindowsServiceName
	}
	if req.BinaryName != nil {
		updates["binary_name"] = *req.BinaryName
	}
	if req.EnvFile != nil {
		updates["env_file"] = *req.EnvFile
	}
	if req.HealthCheckURL != nil {
		updates["health_check_url"] = *req.HealthCheckURL
	}
	if req.IISAppPool != nil {
		updates["iis_app_pool"] = *req.IISAppPool
	}
	if req.IISSiteName != nil {
		updates["iis_site_name"] = *req.IISSiteName
	}
	if req.PublicURL != nil {
		updates["public_url"] = *req.PublicURL
	}
	if req.EnvContent != nil {
		updates["env_content"] = *req.EnvContent
	}

	if len(updates) > 0 {
		if err := h.db.Model(svc).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	var watcher database.Watcher
	h.db.First(&watcher, svc.WatcherID)
	h.syncServiceEnvFile(svc, watcher.InstallDir)

	h.db.Model(&database.Watcher{}).Where("id = ?", svc.WatcherID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	h.db.First(svc, svc.ID)
	c.JSON(http.StatusOK, svc)
}

// DeleteService removes a service.
func (h *Handler) DeleteService(c *gin.Context) {
	svc, err := h.findService(c)
	if err != nil {
		return
	}

	if err := h.db.Delete(svc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.db.Model(&database.Watcher{}).Where("id = ?", svc.WatcherID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	c.JSON(http.StatusOK, MessageResponse{Message: "service deleted"})
}

// RedeployWatcher clears the current version and triggers an immediate check to force a fresh deployment.
func (h *Handler) RedeployWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	// Clear current_version and last_error to force the agent to see a mismatch
	if err := h.db.Model(watcher).Select("current_version", "last_error").Updates(map[string]interface{}{
		"current_version": "",
		"last_error":      "",
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.triggerSync()

	// Trigger immediate check
	select {
	case h.checkTrigger <- watcher.ID:
	default:
	}

	c.JSON(http.StatusAccepted, MessageResponse{Message: "redeploy triggered"})
}

// ── Deploy Logs ───────────────────────────────────────────────

// ListDeployLogs returns deploy history for a watcher.
func (h *Handler) ListDeployLogs(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var logs []database.DeployLog
	if err := h.db.Where("watcher_id = ?", watcher.ID).Order("id desc").Limit(50).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// StreamDeployLogs stream the deployment log for the watcher via SSE.
func (h *Handler) StreamDeployLogs(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	var dLog database.DeployLog
	if err := h.db.Where("watcher_id = ?", watcher.ID).Order("id desc").First(&dLog).Error; err != nil {
		c.SSEvent("error", "No deployment logs found.")
		return
	}

	var lastLen int
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			var currentLog database.DeployLog
			if err := h.db.First(&currentLog, dLog.ID).Error; err != nil {
				return
			}

			if len(currentLog.Logs) > lastLen {
				newText := currentLog.Logs[lastLen:]
				lastLen = len(currentLog.Logs)

				lines := strings.Split(strings.TrimSuffix(newText, "\n"), "\n")
				for _, line := range lines {
					if line != "" {
						c.SSEvent("message", line)
					}
				}
				c.Writer.Flush()
			}

			if currentLog.CompletedAt != nil {
				c.SSEvent("message", "DONE")
				c.Writer.Flush()
				return
			}
		}
	}
}

// ListPollEvents returns the recent polling history for a watcher.
func (h *Handler) ListPollEvents(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "10"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	status := c.Query("status")

	query := h.db.Model(&database.PollEvent{}).Where("watcher_id = ?", watcher.ID)
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	offset := (page - 1) * pageSize
	var events []database.PollEvent
	if err := query.Order("id desc").Limit(pageSize).Offset(offset).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     events,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// ── Helpers ───────────────────────────────────────────────────

func (h *Handler) findWatcher(c *gin.Context) (*database.Watcher, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid watcher id"})
		return nil, err
	}

	var watcher database.Watcher
	if err := h.db.Preload("Services").First(&watcher, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "watcher not found"})
		return nil, err
	}
	return &watcher, nil
}

func (h *Handler) findService(c *gin.Context) (*database.Service, error) {
	// Verify watcher exists
	if _, err := h.findWatcher(c); err != nil {
		return nil, err
	}

	sid, err := strconv.ParseUint(c.Param("sid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := h.db.First(&svc, sid).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	return &svc, nil
}

func withDefault(val, def int) int {
	if val <= 0 {
		return def
	}
	return val
}

// timeNow returns a pointer to the current UTC time.
func timeNow() *time.Time {
	t := time.Now().UTC()
	return &t
}

func (h *Handler) triggerSync() {
	select {
	case h.syncTrigger <- struct{}{}:
	default:
	}
}

type inspectRequest struct {
	RepoURL string `json:"repo_url" binding:"required"`
}

// InspectGitHubRepo uses the configured GitHub token to check a repository's latest release.
func (h *Handler) InspectGitHubRepo(c *gin.Context) {
	var req inspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	logger := agent.NewLogger("") // Temporary logger to stdout
	client := agent.NewGitHubClient(h.githubToken, logger)

	resp, err := client.InspectRepository(c.Request.Context(), req.RepoURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SyncServiceEnv updates the .env content for a service and syncs it to disk.
func (h *Handler) SyncServiceEnv(c *gin.Context) {
	svc, err := h.findService(c)
	if err != nil {
		return
	}

	var req struct {
		EnvContent string `json:"env_content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.db.Model(svc).Update("env_content", req.EnvContent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	var watcher database.Watcher
	h.db.First(&watcher, svc.WatcherID)
	h.syncServiceEnvFile(svc, watcher.InstallDir)

	c.JSON(http.StatusOK, MessageResponse{Message: "Environment file updated and synced"})
}

func (h *Handler) syncServiceEnvFile(svc *database.Service, installDir string) {
	if svc.EnvFile == "" || svc.EnvContent == "" {
		return
	}

	// .env files are usually relative to the install directory
	envPath := filepath.Join(installDir, svc.EnvFile)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
		fmt.Printf("Error creating env dir %s: %v\n", envPath, err)
		return
	}

	if err := os.WriteFile(envPath, []byte(svc.EnvContent), 0600); err != nil {
		fmt.Printf("Error writing env file %s: %v\n", envPath, err)
	}
}
