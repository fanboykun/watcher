package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler holds dependencies for all API endpoints.
type Handler struct {
	db           *gorm.DB
	nssmPath     string
	logDir       string
	version      string
	startTime    time.Time
	checkTrigger chan uint // send watcher ID for immediate poll
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *gorm.DB, nssmPath, logDir, version string, checkTrigger chan uint) *Handler {
	return &Handler{
		db:           db,
		nssmPath:     nssmPath,
		logDir:       logDir,
		version:      version,
		startTime:    time.Now(),
		checkTrigger: checkTrigger,
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
			WindowsServiceName: svcReq.WindowsServiceName,
			BinaryName:         svcReq.BinaryName,
			EnvFile:            svcReq.EnvFile,
			HealthCheckURL:     svcReq.HealthCheckURL,
		}
		if err := h.db.Create(&svc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

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

	if len(updates) > 0 {
		if err := h.db.Model(watcher).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

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
		WindowsServiceName: req.WindowsServiceName,
		BinaryName:         req.BinaryName,
		EnvFile:            req.EnvFile,
		HealthCheckURL:     req.HealthCheckURL,
	}

	if err := h.db.Create(&svc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
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

	if len(updates) > 0 {
		if err := h.db.Model(svc).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

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
	c.JSON(http.StatusOK, MessageResponse{Message: "service deleted"})
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
