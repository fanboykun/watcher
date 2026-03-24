package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
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
	envPath      string
	appCfg       *config.AppConfig
	events       *agent.WatcherEventBus
	startTime    time.Time
	checkTrigger chan uint     // send watcher ID for immediate poll
	syncTrigger  chan struct{} // trigger background agent to sync DB
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *gorm.DB, nssmPath, logDir, version, githubToken, envPath string, appCfg *config.AppConfig, events *agent.WatcherEventBus, checkTrigger chan uint, syncTrigger chan struct{}) *Handler {
	return &Handler{
		db:           db,
		nssmPath:     nssmPath,
		logDir:       logDir,
		version:      version,
		githubToken:  githubToken,
		envPath:      envPath,
		appCfg:       appCfg,
		events:       events,
		startTime:    time.Now(),
		checkTrigger: checkTrigger,
		syncTrigger:  syncTrigger,
	}
}

// ── Watcher CRUD ──────────────────────────────────────────────

// ListWatchers returns all watchers with their services and current state.
func (h *Handler) ListWatchers(c *gin.Context) {
	var watchers []database.Watcher
	if err := h.db.Preload("Services").Preload("Services.ConfigFiles").Find(&watchers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	for i := range watchers {
		enrichWatcherSecrets(&watchers[i])
	}
	c.JSON(http.StatusOK, watchers)
}

// GetWatcher returns a single watcher with services.
func (h *Handler) GetWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return // response already sent
	}
	enrichWatcherSecrets(watcher)
	c.JSON(http.StatusOK, watcher)
}

// StreamWatcherEvents streams watcher state events (SSE) for real-time UI updates.
func (h *Handler) StreamWatcherEvents(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	if h.events == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "watcher events are not configured"})
		return
	}

	ch, unsubscribe := h.events.Subscribe(watcher.ID)
	defer unsubscribe()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	// Emit one immediate message so clients don't wait for first heartbeat/data.
	c.SSEvent("message", agent.WatcherEvent{
		Type:      "connected",
		WatcherID: watcher.ID,
		Timestamp: time.Now().UTC(),
	})
	c.Writer.Flush()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case ev, ok := <-ch:
			if !ok {
				return false
			}
			c.SSEvent("message", ev)
			return true
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			return true
		}
	})
}

// CreateWatcher creates a new watcher entry with optional inline services.
func (h *Handler) CreateWatcher(c *gin.Context) {
	var req CreateWatcherRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	watcher := database.Watcher{
		Name:                  req.Name,
		ServiceName:           req.ServiceName,
		MetadataURL:           req.MetadataURL,
		DeploymentEnvironment: strings.TrimSpace(req.DeploymentEnvironment),
		GitHubToken:           strings.TrimSpace(req.GitHubToken),
		CheckIntervalSec:      withDefault(req.CheckIntervalSec, 60),
		DownloadRetries:       withDefault(req.DownloadRetries, 3),
		InstallDir:            req.InstallDir,
		HcEnabled:             req.HcEnabled,
		HcURL:                 req.HcURL,
		HcRetries:             withDefault(req.HcRetries, 10),
		HcIntervalSec:         withDefault(req.HcIntervalSec, 3),
		HcTimeoutSec:          withDefault(req.HcTimeoutSec, 5),
		Paused:                req.Paused,
		MaxKeptVersions:       withDefault(req.MaxKeptVersions, 3),
		Status:                "unknown",
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
	enrichWatcherSecrets(&watcher)
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
	if req.DeploymentEnvironment != nil {
		updates["deployment_environment"] = strings.TrimSpace(*req.DeploymentEnvironment)
	}
	if req.GitHubToken != nil {
		updates["github_token"] = strings.TrimSpace(*req.GitHubToken)
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
	if req.MaxKeptVersions != nil {
		updates["max_kept_versions"] = *req.MaxKeptVersions
	}
	if req.MaxKeptVersions != nil {
		updates["max_kept_versions"] = *req.MaxKeptVersions
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
	enrichWatcherSecrets(watcher)
	c.JSON(http.StatusOK, watcher)
}

// DeleteWatcher soft-deletes a watcher and its services (cascade).
func (h *Handler) DeleteWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if err := h.cleanupWatcherServices(watcher); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.removeWatcherInstallDir(watcher); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
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

func (h *Handler) cleanupWatcherServices(watcher *database.Watcher) error {
	if watcher == nil || runtime.GOOS != "windows" {
		return nil
	}

	for _, svc := range watcher.Services {
		if defaultServiceType(svc.ServiceType) != "nssm" {
			continue
		}
		name := strings.TrimSpace(svc.WindowsServiceName)
		if name == "" {
			continue
		}

		if err := h.stopNSSMService(name); err != nil {
			return err
		}
		if err := h.removeNSSMService(name); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) removeWatcherInstallDir(watcher *database.Watcher) error {
	if watcher == nil {
		return nil
	}

	installDir := strings.TrimSpace(watcher.InstallDir)
	if installDir == "" {
		return nil
	}

	info, err := os.Stat(installDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect watcher install dir %s: %w", installDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("watcher install path is not a directory: %s", installDir)
	}

	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("failed to delete watcher install dir %s: %w", installDir, err)
	}
	return nil
}

func (h *Handler) stopNSSMService(name string) error {
	out, err := exec.Command(h.nssmPath, "stop", name, "confirm").CombinedOutput()
	if err == nil || isServiceMissingOutput(string(out)) || isServiceStoppedOutput(string(out)) {
		return nil
	}
	return fmt.Errorf("failed to stop service %s before watcher deletion: %s", name, strings.TrimSpace(string(out)))
}

func (h *Handler) removeNSSMService(name string) error {
	out, err := exec.Command(h.nssmPath, "remove", name, "confirm").CombinedOutput()
	if err == nil || isServiceMissingOutput(string(out)) {
		return nil
	}
	return fmt.Errorf("failed to remove service %s before watcher deletion: %s", name, strings.TrimSpace(string(out)))
}

func isServiceMissingOutput(out string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(out))
	return strings.Contains(normalized, "DOES NOT EXIST") ||
		strings.Contains(normalized, "SERVICE_DOES_NOT_EXIST") ||
		strings.Contains(normalized, "OPENSERVICE(): THE SPECIFIED SERVICE DOES NOT EXIST")
}

func isServiceStoppedOutput(out string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(out))
	return strings.Contains(normalized, "SERVICE_STOPPED") ||
		strings.Contains(normalized, "SERVICE_NOT_ACTIVE")
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
	if err := h.db.Preload("ConfigFiles").Where("watcher_id = ?", watcher.ID).Find(&services).Error; err != nil {
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

	if len(req.ConfigFiles) > 0 {
		configFiles := make([]database.ServiceConfigFile, 0, len(req.ConfigFiles))
		for _, file := range req.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" {
				continue
			}
			configFiles = append(configFiles, database.ServiceConfigFile{
				ServiceID: svc.ID,
				FilePath:  strings.TrimSpace(file.FilePath),
				Content:   file.Content,
			})
		}
		if len(configFiles) > 0 {
			if err := h.db.Create(&configFiles).Error; err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
				return
			}
			svc.ConfigFiles = configFiles
		}
	}

	h.syncServiceFiles(&svc, watcher.InstallDir)

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
	if req.ConfigFiles != nil {
		if err := h.db.Where("service_id = ?", svc.ID).Delete(&database.ServiceConfigFile{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		configFiles := make([]database.ServiceConfigFile, 0, len(*req.ConfigFiles))
		for _, file := range *req.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" {
				continue
			}
			configFiles = append(configFiles, database.ServiceConfigFile{
				ServiceID: svc.ID,
				FilePath:  strings.TrimSpace(file.FilePath),
				Content:   file.Content,
			})
		}
		if len(configFiles) > 0 {
			if err := h.db.Create(&configFiles).Error; err != nil {
				c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
				return
			}
		}
	}

	h.db.Preload("ConfigFiles").First(svc, svc.ID)
	h.syncServiceFiles(svc, watcher.InstallDir)

	h.db.Model(&database.Watcher{}).Where("id = ?", svc.WatcherID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	h.db.Preload("ConfigFiles").First(svc, svc.ID)
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

	// Guard: do not queue redeploy while another deploy is still in progress.
	var active database.DeployLog
	if err := h.db.Where("watcher_id = ? AND completed_at IS NULL", watcher.ID).Order("id desc").First(&active).Error; err == nil {
		apiBaseURL := ""
		if h.appCfg != nil {
			apiBaseURL = h.appCfg.APIBaseURL
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":         "deployment already in progress",
			"deploy_log_id": active.ID,
			"log_url":       buildWatcherLogURL(apiBaseURL, watcher.ID, active.ID),
		})
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

	now := time.Now().UTC()
	queuedVersion := strings.TrimSpace(watcher.CurrentVersion)
	if queuedVersion == "" {
		queuedVersion = "pending"
	}
	dlog := database.DeployLog{
		WatcherID:   watcher.ID,
		TriggeredBy: "manual",
		Version:     queuedVersion,
		FromVersion: watcher.CurrentVersion,
		Status:      string(agent.StatusDeploying),
		StartedAt:   &now,
		Logs:        "redeploy: queued manual redeploy request",
	}
	if err := h.db.Create(&dlog).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if h.events != nil {
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventDeployStarted,
			Data: map[string]any{
				"deploy_log_id": dlog.ID,
				"version":       queuedVersion,
				"from_version":  watcher.CurrentVersion,
				"triggered_by":  "manual",
			},
		})
	}

	h.triggerSync()

	// Trigger immediate check
	select {
	case h.checkTrigger <- watcher.ID:
	default:
	}

	apiBaseURL := ""
	if h.appCfg != nil {
		apiBaseURL = h.appCfg.APIBaseURL
	}
	c.JSON(http.StatusAccepted, gin.H{
		"message":       "redeploy triggered",
		"deploy_log_id": dlog.ID,
		"log_url":       buildWatcherLogURL(apiBaseURL, watcher.ID, dlog.ID),
	})
}

// ── Deploy Logs ───────────────────────────────────────────────

// ListDeployLogs returns deploy history for a watcher.
func (h *Handler) ListDeployLogs(c *gin.Context) {
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

	query := h.db.Model(&database.DeployLog{}).Where("watcher_id = ?", watcher.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	offset := (page - 1) * pageSize
	var logs []database.DeployLog
	if err := query.Order("id desc").Limit(pageSize).Offset(offset).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":     logs,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// StreamDeployLog streams one deployment log by ID via SSE.
func (h *Handler) StreamDeployLog(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	did, err := strconv.ParseUint(c.Param("did"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid deploy log id"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	var dLog database.DeployLog
	if err := h.db.Where("id = ? AND watcher_id = ?", did, watcher.ID).First(&dLog).Error; err != nil {
		c.SSEvent("error", "No deployment logs found.")
		c.Writer.Flush()
		return
	}

	lastLen := len(dLog.Logs)
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

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
		case <-heartbeat.C:
			fmt.Fprint(c.Writer, ": ping\n\n")
			c.Writer.Flush()
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
	if err := h.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, id).Error; err != nil {
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
	if err := h.db.Preload("ConfigFiles").First(&svc, sid).Error; err != nil {
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

func compareSemver(a, b string) int {
	parse := func(v string) [3]int {
		var out [3]int
		v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
		parts := strings.Split(v, ".")
		for i := 0; i < 3 && i < len(parts); i++ {
			fmt.Sscanf(parts[i], "%d", &out[i])
		}
		return out
	}

	pa := parse(a)
	pb := parse(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

func buildWatcherLogURL(apiBaseURL string, watcherID, deployLogID uint) string {
	base := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/")
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/watchers/%d/logs/%d", base, watcherID, deployLogID)
}

func enrichWatcherSecrets(w *database.Watcher) {
	if w == nil {
		return
	}
	token := strings.TrimSpace(w.GitHubToken)
	w.HasGitHubToken = token != ""
	w.GitHubTokenMasked = maskToken(token)
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

// touchWatchersUpdatedAt forces the agent to recreate repo watchers on next sync.
func (h *Handler) touchWatchersUpdatedAt() {
	h.db.Model(&database.Watcher{}).Where("1 = 1").UpdateColumn("updated_at", time.Now())
}

type inspectRequest struct {
	RepoURL     string `json:"repo_url" binding:"required"`
	GitHubToken string `json:"github_token"`
}

// InspectGitHubRepo uses the configured GitHub token to check a repository's latest release.
func (h *Handler) InspectGitHubRepo(c *gin.Context) {
	var req inspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	token := strings.TrimSpace(req.GitHubToken)
	if token == "" {
		token = h.githubToken
	}
	logger := agent.NewLogger("") // Temporary logger to stdout
	client := agent.NewGitHubClient(token, logger)

	resp, err := client.InspectRepository(c.Request.Context(), req.RepoURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SyncServiceEnv updates the .env content for a service and syncs it to disk.
func (h *Handler) SyncServiceEnv(c *gin.Context) {
	svc, err := h.findServiceByID(c)
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
	h.db.Preload("ConfigFiles").First(svc, svc.ID)
	h.syncServiceFiles(svc, watcher.InstallDir)

	c.JSON(http.StatusOK, MessageResponse{Message: "Environment file updated and synced"})
}

func (h *Handler) syncServiceFiles(svc *database.Service, installDir string) {
	if svc.EnvFile != "" && svc.EnvContent != "" {
		h.writeServiceFile(installDir, svc.EnvFile, svc.EnvContent)
	}
	for _, file := range svc.ConfigFiles {
		if strings.TrimSpace(file.FilePath) == "" {
			continue
		}
		h.writeServiceFile(installDir, file.FilePath, file.Content)
	}
}

func (h *Handler) writeServiceFile(installDir, relativePath, content string) {
	targetPath := filepath.Join(installDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		fmt.Printf("Error creating config dir %s: %v\n", targetPath, err)
		return
	}
	if err := os.WriteFile(targetPath, []byte(content), 0600); err != nil {
		fmt.Printf("Error writing config file %s: %v\n", targetPath, err)
	}
}

// ── Deploy Log Detail ─────────────────────────────────────────

// GetDeployLog returns a single deploy log by ID (URL-able for GitHub Deployment API log_url).
func (h *Handler) GetDeployLog(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	did, err := strconv.ParseUint(c.Param("did"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid deploy log id"})
		return
	}

	var dlog database.DeployLog
	if err := h.db.Where("id = ? AND watcher_id = ?", did, watcher.ID).First(&dlog).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "deploy log not found"})
		return
	}

	c.JSON(http.StatusOK, dlog)
}

// ── Version Management ────────────────────────────────────────

// ListAvailableVersions returns on-disk release versions available for rollback.
func (h *Handler) ListAvailableVersions(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	versions, err := agent.ListAvailableVersions(watcher.InstallDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	currentVersion := strings.TrimSpace(watcher.CurrentVersion)
	if currentVersion != "" {
		for i := range versions {
			versions[i].IsCurrent = versions[i].Version == currentVersion
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"watcher_id":      watcher.ID,
		"current_version": watcher.CurrentVersion,
		"versions":        versions,
	})
}

// RollbackWatcher rolls back a watcher to a specific version that exists on disk.
func (h *Handler) RollbackWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify the version exists on disk
	releaseDir := filepath.Join(watcher.InstallDir, "releases", req.Version)
	if _, err := os.Stat(releaseDir); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("version %s not found on disk at %s", req.Version, releaseDir),
		})
		return
	}

	// Create deploy log first and process rollback asynchronously so API can return immediately.
	wcfg := agent.WatcherConfigFromDB(watcher)
	logger := agent.NewLogger(wcfg.Name)

	now := time.Now().UTC()
	dlog := database.DeployLog{
		WatcherID:   watcher.ID,
		TriggeredBy: "manual",
		Version:     req.Version,
		FromVersion: watcher.CurrentVersion,
		Status:      "rollback",
		StartedAt:   &now,
	}
	if err := h.db.Create(&dlog).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if h.events != nil {
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventDeployStarted,
			Data: map[string]any{
				"deploy_log_id": dlog.ID,
				"version":       req.Version,
				"from_version":  watcher.CurrentVersion,
				"triggered_by":  "manual",
			},
		})
	}

	reportGitHub := true
	if req.ReportGitHub != nil {
		reportGitHub = *req.ReportGitHub
	}
	if h.appCfg != nil && !h.appCfg.GitHubDeployEnabled {
		reportGitHub = false
	}

	_ = h.db.Model(watcher).Updates(map[string]any{
		"status": "rollback",
	})

	h.triggerSync()

	apiBaseURL := ""
	if h.appCfg != nil {
		apiBaseURL = h.appCfg.APIBaseURL
	}
	logURL := buildWatcherLogURL(apiBaseURL, watcher.ID, dlog.ID)
	go h.runRollback(watcher, dlog.ID, req.Version, watcher.CurrentVersion, reportGitHub, logger, wcfg)

	c.JSON(http.StatusAccepted, gin.H{
		"message":       fmt.Sprintf("rollback to %s started", req.Version),
		"version":       req.Version,
		"deploy_log_id": dlog.ID,
		"log_url":       logURL,
	})
}

func (h *Handler) runRollback(watcher *database.Watcher, deployLogID uint, targetVersion, previousVersion string, reportGitHub bool, logger *agent.Logger, wcfg *agent.WatcherConfig) {
	startedAt := time.Now().UTC()
	appendRollbackLog := func(text string) {
		_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).
			UpdateColumn("logs", gorm.Expr("COALESCE(logs, '') || ?", text+"\n")).Error
	}

	deployer := agent.NewDeployer(wcfg, h.nssmPath, logger, appendRollbackLog)
	appendRollbackLog(fmt.Sprintf("rollback: started target=%s from=%s", targetVersion, previousVersion))

	if err := deployer.Rollback(context.Background(), targetVersion); err != nil {
		completed := time.Now().UTC()
		durationMs := completed.Sub(startedAt).Milliseconds()
		_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Updates(map[string]any{
			"status":       "failed",
			"error":        err.Error(),
			"completed_at": &completed,
			"duration_ms":  durationMs,
		}).Error
		_ = h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).Updates(map[string]any{
			"status":     "failed",
			"last_error": err.Error(),
		}).Error
		if h.events != nil {
			h.events.Publish(watcher.ID, agent.WatcherEvent{
				Type: agent.EventDeployFinished,
				Data: map[string]any{
					"deploy_log_id": deployLogID,
					"status":        "failed",
					"error":         err.Error(),
				},
			})
		}
		h.triggerSync()
		return
	}

	completed := time.Now().UTC()
	durationMs := completed.Sub(startedAt).Milliseconds()
	maxIgnored := ""
	if compareSemver(targetVersion, previousVersion) < 0 {
		maxIgnored = previousVersion
	}
	_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Updates(map[string]any{
		"status":       "healthy",
		"completed_at": &completed,
		"duration_ms":  durationMs,
	}).Error
	_ = h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).Updates(map[string]any{
		"current_version":     targetVersion,
		"max_ignored_version": maxIgnored,
		"status":              "healthy",
		"last_deployed":       &completed,
		"last_error":          "",
	}).Error
	if h.events != nil {
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventDeployFinished,
			Data: map[string]any{
				"deploy_log_id": deployLogID,
				"status":        "healthy",
				"version":       targetVersion,
			},
		})
		h.events.Publish(watcher.ID, agent.WatcherEvent{
			Type: agent.EventVersionChanged,
			Data: map[string]any{
				"version": targetVersion,
			},
		})
	}

	if reportGitHub {
		token := strings.TrimSpace(watcher.GitHubToken)
		if token == "" {
			token = strings.TrimSpace(h.githubToken)
		}
		env := strings.TrimSpace(watcher.DeploymentEnvironment)
		if env == "" && h.appCfg != nil {
			env = strings.TrimSpace(h.appCfg.Environment)
		}
		if token == "" || env == "" {
			appendRollbackLog("github_deployment: skipped for rollback (missing token or environment)")
		} else {
			owner, repo, parseErr := agent.ParseGitHubURL(watcher.MetadataURL)
			if parseErr != nil {
				appendRollbackLog("github_deployment: rollback parse repo failed: " + parseErr.Error())
			} else {
				gh := agent.NewGitHubClient(token, logger)
				desc := fmt.Sprintf("Manual rollback %s to %s", watcher.ServiceName, targetVersion)
				appendRollbackLog(fmt.Sprintf("github_deployment: rollback create deployment repo=%s/%s ref=%s env=%q", owner, repo, targetVersion, env))
				deploymentID, createErr := gh.CreateDeployment(context.Background(), owner, repo, targetVersion, env, desc)
				if createErr != nil {
					appendRollbackLog("github_deployment: rollback create deployment failed: " + createErr.Error())
				} else {
					_ = h.db.Model(&database.DeployLog{}).Where("id = ?", deployLogID).Update("github_deployment_id", deploymentID).Error
					apiBaseURL := ""
					if h.appCfg != nil {
						apiBaseURL = h.appCfg.APIBaseURL
					}
					logURL := buildWatcherLogURL(apiBaseURL, watcher.ID, deployLogID)
					if statusErr := gh.UpdateDeploymentStatus(context.Background(), owner, repo, deploymentID, "success", logURL, desc); statusErr != nil {
						appendRollbackLog("github_deployment: rollback status=success failed: " + statusErr.Error())
					} else {
						appendRollbackLog(fmt.Sprintf("github_deployment: rollback status=success deployment_id=%d", deploymentID))
					}
				}
			}
		}
	} else {
		appendRollbackLog("github_deployment: skipped for rollback by request/config")
	}

	h.triggerSync()
}

// ResumeWatcherUpdates clears the max_ignored_version flag so polling updates resume.
func (h *Handler) ResumeWatcherUpdates(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	h.db.Model(watcher).Update("max_ignored_version", "")
	h.triggerSync()

	c.JSON(http.StatusOK, gin.H{"message": "auto-deploy resumed"})
}

// DeleteWatcherVersion removes a specific version directory from disk
func (h *Handler) DeleteWatcherVersion(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")

	var watcher database.Watcher
	if err := h.db.First(&watcher, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "watcher not found"})
		return
	}

	if err := agent.DeleteVersion(watcher.InstallDir, version); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("deleted version %s", version),
		"version": version,
	})
}
