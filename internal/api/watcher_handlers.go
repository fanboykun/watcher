package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"errors"
	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) ListWatchers(c *gin.Context) {
	var watchers []database.Watcher
	if err := h.db.Preload("Services").Preload("Services.ConfigFiles").Find(&watchers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.enrichLegacyPolling(watchers)
	for i := range watchers {
		normalizeWatcherServices(&watchers[i])
		enrichWatcherSecrets(&watchers[i])
		h.enrichPolling(&watchers[i])
	}
	c.JSON(http.StatusOK, watchers)
}

// GetWatcher returns a single watcher with services.
func (h *Handler) GetWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return // response already sent
	}
	legacy := []database.Watcher{*watcher}
	h.enrichLegacyPolling(legacy)
	*watcher = legacy[0]
	normalizeWatcherServices(watcher)
	enrichWatcherSecrets(watcher)
	h.enrichPolling(watcher)
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
	if err := validateOptionalWebhookSigningSecret(req.WebhookSigningSecret); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := validateResolvedWebhookConfig(req.WebhookEnabled, strings.TrimSpace(req.WebhookURL), strings.TrimSpace(req.WebhookSigningSecret), h.appCfg); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	autoDeploy := true
	if req.AutoDeploy != nil {
		autoDeploy = *req.AutoDeploy
	}

	watcher := database.Watcher{
		Name:                            req.Name,
		ServiceName:                     req.ServiceName,
		MetadataURL:                     req.MetadataURL,
		ReleaseRef:                      defaultReleaseRef(req.ReleaseRef),
		DeploymentEnvironment:           strings.TrimSpace(req.DeploymentEnvironment),
		GitHubToken:                     strings.TrimSpace(req.GitHubToken),
		CheckIntervalSec:                withDefault(req.CheckIntervalSec, 60),
		DownloadRetries:                 withDefault(req.DownloadRetries, 3),
		InstallDir:                      req.InstallDir,
		HcEnabled:                       req.HcEnabled,
		HcURL:                           req.HcURL,
		HcRetries:                       withDefault(req.HcRetries, 10),
		HcIntervalSec:                   withDefault(req.HcIntervalSec, 3),
		HcTimeoutSec:                    withDefault(req.HcTimeoutSec, 5),
		Paused:                          req.Paused,
		AutoDeploy:                      autoDeploy,
		MaxKeptVersions:                 withDefault(req.MaxKeptVersions, 3),
		WebhookEnabled:                  req.WebhookEnabled,
		WebhookURL:                      strings.TrimSpace(req.WebhookURL),
		WebhookSigningSecret:            strings.TrimSpace(req.WebhookSigningSecret),
		WebhookAutoPauseEnabledOverride: req.WebhookAutoPauseEnabledOverride,
		WebhookAutoPauseAfterFailures:   req.WebhookAutoPauseAfterFailures,
		NotifyVersionFound:              req.NotifyVersionFound,
		NotifyDeploymentSucceeded:       req.NotifyDeploymentSucceeded,
		NotifyDeploymentFailed:          req.NotifyDeploymentFailed,
		NotifyRollbackSucceeded:         req.NotifyRollbackSucceeded,
		NotifyRollbackFailed:            req.NotifyRollbackFailed,
		NotifyServiceHealthChanged:      req.NotifyServiceHealthChanged,
		Status:                          "unknown",
	}

	// Create watcher
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&watcher).Error; err != nil {
			return err
		}
		// GORM substitutes the true default for a zero bool during Create.
		if !autoDeploy {
			return tx.Model(&watcher).UpdateColumn("auto_deploy", false).Error
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	// Create inline services if provided
	for _, svcReq := range req.Services {
		svc := database.Service{
			WatcherID:          watcher.ID,
			ServiceType:        normalizeServiceType(svcReq.ServiceType),
			WindowsServiceName: svcReq.WindowsServiceName,
			BinaryName:         svcReq.BinaryName,
			StartArguments:     svcReq.StartArguments,
			EnvFile:            svcReq.EnvFile,
			HealthCheckURL:     svcReq.HealthCheckURL,
			IISAppKind:         normalizeIISAppKind(svcReq.IISAppKind, svcReq.IISManagedRuntime),
			IISAppPool:         svcReq.IISAppPool,
			IISSiteName:        svcReq.IISSiteName,
			IISManagedRuntime:  resolvedIISManagedRuntime(svcReq.IISAppKind, svcReq.IISManagedRuntime),
			PublicURL:          svcReq.PublicURL,
			EnvContent:         svcReq.EnvContent,
		}
		if err := validateServicePayload(&svc); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		if err := h.db.Create(&svc).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		if len(svcReq.ConfigFiles) > 0 {
			configFiles := make([]database.ServiceConfigFile, 0, len(svcReq.ConfigFiles))
			for _, file := range svcReq.ConfigFiles {
				if strings.TrimSpace(file.FilePath) == "" {
					continue
				}
				configFiles = append(configFiles, database.ServiceConfigFile{
					ServiceID: svc.ID,
					FilePath:  strings.TrimSpace(file.FilePath),
					Target:    normalizeConfigFileTarget(file.Target),
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
	}

	h.triggerSync()

	// Reload with services
	h.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, watcher.ID)
	normalizeWatcherServices(&watcher)
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
	if req.WebhookSigningSecret != nil {
		if err := validateOptionalWebhookSigningSecret(*req.WebhookSigningSecret); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
	}

	nextWebhookEnabled := watcher.WebhookEnabled
	if req.WebhookEnabled != nil {
		nextWebhookEnabled = *req.WebhookEnabled
	}
	nextWebhookURL := watcher.WebhookURL
	if req.WebhookURL != nil {
		nextWebhookURL = strings.TrimSpace(*req.WebhookURL)
	}
	nextWebhookSecret := watcher.WebhookSigningSecret
	if req.WebhookSigningSecret != nil {
		nextWebhookSecret = strings.TrimSpace(*req.WebhookSigningSecret)
	}
	if err := validateResolvedWebhookConfig(nextWebhookEnabled, nextWebhookURL, nextWebhookSecret, h.appCfg); err != nil {
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
	if req.ReleaseRef != nil {
		updates["release_ref"] = defaultReleaseRef(*req.ReleaseRef)
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
	if req.AutoDeploy != nil {
		updates["auto_deploy"] = *req.AutoDeploy
	}
	if req.InterceptNextRelease != nil {
		updates["intercept_next_release"] = *req.InterceptNextRelease
	}
	if req.MaxKeptVersions != nil {
		updates["max_kept_versions"] = *req.MaxKeptVersions
	}
	if req.WebhookEnabled != nil {
		updates["webhook_enabled"] = *req.WebhookEnabled
	}
	if req.WebhookURL != nil {
		updates["webhook_url"] = strings.TrimSpace(*req.WebhookURL)
	}
	if req.WebhookSigningSecret != nil {
		updates["webhook_signing_secret"] = strings.TrimSpace(*req.WebhookSigningSecret)
	}
	if req.WebhookAutoPauseEnabledOverride != nil {
		updates["webhook_auto_pause_enabled_override"] = *req.WebhookAutoPauseEnabledOverride
	}
	if req.WebhookAutoPauseAfterFailures != nil {
		updates["webhook_auto_pause_after_failures"] = *req.WebhookAutoPauseAfterFailures
	}
	if req.NotifyVersionFound != nil {
		updates["notify_version_found"] = *req.NotifyVersionFound
	}
	if req.NotifyDeploymentSucceeded != nil {
		updates["notify_deployment_succeeded"] = *req.NotifyDeploymentSucceeded
	}
	if req.NotifyDeploymentFailed != nil {
		updates["notify_deployment_failed"] = *req.NotifyDeploymentFailed
	}
	if req.NotifyRollbackSucceeded != nil {
		updates["notify_rollback_succeeded"] = *req.NotifyRollbackSucceeded
	}
	if req.NotifyRollbackFailed != nil {
		updates["notify_rollback_failed"] = *req.NotifyRollbackFailed
	}
	if req.NotifyServiceHealthChanged != nil {
		updates["notify_service_health_changed"] = *req.NotifyServiceHealthChanged
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
	h.enrichPolling(watcher)
	c.JSON(http.StatusOK, watcher)
}

// DeleteWatcher soft-deletes a watcher and its services (cascade).
func (h *Handler) DeleteWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if err := h.cleanupWatcherServices(c.Request.Context(), watcher); err != nil {
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

func (h *Handler) InterceptRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	var req struct {
		Intercept bool `json:"intercept_next_release"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.db.Model(watcher).UpdateColumn("intercept_next_release", req.Intercept).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.triggerSync()
	c.JSON(http.StatusOK, MessageResponse{Message: "Intercept setting updated"})
}

func (h *Handler) ApproveRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	var req struct {
		Version string `json:"version" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if watcher.PendingCatalogID != 0 {
		if watcher.PendingVersion != req.Version {
			c.JSON(http.StatusConflict, ErrorResponse{Error: "The selected candidate changed"})
			return
		}
		h.queueCatalogDeploy(c, watcher, watcher.PendingCatalogID)
		return
	}
	result := h.db.Model(watcher).Where("status = ? AND pending_version = ?", "pending_approval", req.Version).
		UpdateColumns(map[string]any{"status": "approved", "approved_version": req.Version})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "The pending release changed; refresh before approving"})
		return
	}

	_, _ = h.queueCheck(c, watcher.ID)

	c.JSON(http.StatusOK, MessageResponse{Message: "Release approved and deployment triggered"})
}

// DiscardRelease discards a pending release candidate and unblocks polling.
func (h *Handler) DiscardRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	if watcher.Status != "pending_approval" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "No release pending approval"})
		return
	}

	discardedVersion := watcher.PendingVersion
	status := "healthy"
	if watcher.CurrentVersion == "" {
		status = "unknown"
	}

	updates := map[string]interface{}{
		"status":                 status,
		"pending_version":        "",
		"intercept_next_release": false,
	}
	if discardedVersion != "" {
		updates["max_ignored_version"] = discardedVersion
	}

	updates["approved_version"] = ""
	updates["pending_catalog_id"] = 0
	err = h.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(watcher).Where("status = ? AND pending_version = ?", "pending_approval", discardedVersion).UpdateColumns(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if discardedVersion != "" {
			if err := tx.Where("service_id IN (SELECT id FROM services WHERE watcher_id = ?) AND target_version = ?", watcher.ID, discardedVersion).Delete(&database.ServiceConfigRevision{}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&database.PollEvent{WatcherID: watcher.ID, CheckedAt: time.Now().UTC(), Status: "discarded", RemoteVersion: discardedVersion, Error: "Release candidate discarded by operator"}).Error
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusConflict
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}

	h.triggerSync()
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("Release candidate %s discarded", discardedVersion)})
}

// GetWatcherCandidate retrieves candidate configuration for all services under a watcher.
func (h *Handler) GetWatcherCandidate(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	target := strings.TrimSpace(c.Query("target"))
	if target == "" {
		if watcher.PendingVersion != "" {
			target = watcher.PendingVersion
		} else {
			target = "next"
		}
	}

	var services []database.Service
	if err := h.db.Where("watcher_id = ?", watcher.ID).Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	serviceIDs := make([]uint, 0, len(services))
	for _, svc := range services {
		serviceIDs = append(serviceIDs, svc.ID)
	}
	var revisions []database.ServiceConfigRevision
	if len(serviceIDs) > 0 {
		if err := h.db.Where("service_id IN ? AND target_version IN ?", serviceIDs, []string{target, "next"}).Find(&revisions).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	revisionsByService := make(map[uint]database.ServiceConfigRevision)
	for _, rev := range revisions {
		previous, exists := revisionsByService[rev.ServiceID]
		if !exists || previous.TargetVersion != target {
			revisionsByService[rev.ServiceID] = rev
		}
	}
	serviceInfos := make([]ServiceCandidateInfo, 0, len(services))
	for _, svc := range services {
		name := svc.WindowsServiceName
		if name == "" {
			name = svc.IISSiteName
		}
		if name == "" {
			name = svc.IISAppPool
		}
		if name == "" {
			name = svc.BinaryName
		}
		if name == "" {
			name = fmt.Sprintf("Service %d", svc.ID)
		}

		rev, hasCandidate := revisionsByService[svc.ID]
		candidateEnv := svc.EnvContent
		if hasCandidate {
			candidateEnv = rev.EnvContent
		}

		serviceInfos = append(serviceInfos, ServiceCandidateInfo{
			ServiceID:       svc.ID,
			ServiceName:     name,
			ServiceType:     svc.ServiceType,
			ActiveEnv:       svc.EnvContent,
			CandidateEnv:    candidateEnv,
			HasCandidateEnv: hasCandidate,
			IsModified:      candidateEnv != svc.EnvContent,
		})
	}

	c.JSON(http.StatusOK, WatcherCandidateResponse{
		HasPendingRelease:    watcher.Status == "pending_approval",
		TargetVersion:        target,
		Status:               watcher.Status,
		InterceptNextRelease: watcher.InterceptNextRelease,
		AutoDeploy:           watcher.AutoDeploy,
		Services:             serviceInfos,
	})
}

// UpdateWatcherCandidate saves candidate configuration revisions across services for a watcher.
func (h *Handler) UpdateWatcherCandidate(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var req UpdateWatcherCandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	target := strings.TrimSpace(req.TargetVersion)
	if target == "" {
		if watcher.PendingVersion != "" {
			target = watcher.PendingVersion
		} else {
			target = "next"
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Services {
			var svc database.Service
			if err := tx.Where("id = ? AND watcher_id = ?", item.ServiceID, watcher.ID).First(&svc).Error; err != nil {
				return err
			}
			if err := saveServiceConfigRevision(tx, svc.ID, target, item.EnvContent); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusBadRequest
		}
		c.JSON(status, ErrorResponse{Error: "Could not save candidate configuration: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: "Candidate configuration saved successfully"})
}

func (h *Handler) enrichPolling(w *database.Watcher) {
	w.PollingActivity, w.PollStartedAt = h.polling.Snapshot(w.ID, w.Paused)
}

// Preserve historical results until the first cycle stores a completion summary.
func (h *Handler) enrichLegacyPolling(watchers []database.Watcher) {
	ids := make([]uint, 0)
	for _, w := range watchers {
		if w.LastPollAt == nil {
			ids = append(ids, w.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	var events []database.PollEvent
	if err := h.db.Where("watcher_id IN ? AND id IN (SELECT MAX(id) FROM poll_events GROUP BY watcher_id)", ids).Find(&events).Error; err != nil {
		return
	}
	latest := make(map[uint]database.PollEvent, len(events))
	for _, event := range events {
		latest[event.WatcherID] = event
	}
	for i := range watchers {
		if watchers[i].LastPollAt != nil {
			continue
		}
		if event, ok := latest[watchers[i].ID]; ok {
			at := event.CheckedAt
			watchers[i].LastPollStatus, watchers[i].LastPollError, watchers[i].LastPollID, watchers[i].LastPollAt = event.Status, event.Error, event.PollID, &at
		}
	}
}
