package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

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

	services := make([]database.Service, 0)
	if err := h.db.Preload("ConfigFiles").Where("watcher_id = ?", watcher.ID).Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, normalizeServices(services))
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
		ServiceType:        normalizeServiceType(req.ServiceType),
		WindowsServiceName: req.WindowsServiceName,
		BinaryName:         req.BinaryName,
		StartArguments:     req.StartArguments,
		EnvFile:            req.EnvFile,
		HealthCheckURL:     req.HealthCheckURL,
		IISAppKind:         normalizeIISAppKind(req.IISAppKind, req.IISManagedRuntime),
		IISAppPool:         req.IISAppPool,
		IISSiteName:        req.IISSiteName,
		IISManagedRuntime:  resolvedIISManagedRuntime(req.IISAppKind, req.IISManagedRuntime),
		PublicURL:          req.PublicURL,
		EnvContent:         req.EnvContent,
	}
	if err := validateServicePayload(&svc); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
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

	if err := h.syncServiceFiles(&svc, watcher.InstallDir); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.refreshActiveConfigSnapshot(watcher.ID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	normalizeServiceCollections(&svc)
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
		updates["service_type"] = normalizeServiceType(*req.ServiceType)
	}
	if req.WindowsServiceName != nil {
		updates["windows_service_name"] = *req.WindowsServiceName
	}
	if req.BinaryName != nil {
		updates["binary_name"] = *req.BinaryName
	}
	if req.StartArguments != nil {
		updates["start_arguments"] = *req.StartArguments
	}
	if req.EnvFile != nil {
		updates["env_file"] = *req.EnvFile
	}
	if req.HealthCheckURL != nil {
		updates["health_check_url"] = *req.HealthCheckURL
	}
	if req.IISAppKind != nil {
		updates["iis_app_kind"] = normalizeIISAppKind(*req.IISAppKind, svc.IISManagedRuntime)
	}
	if req.IISAppPool != nil {
		updates["iis_app_pool"] = *req.IISAppPool
	}
	if req.IISSiteName != nil {
		updates["iis_site_name"] = *req.IISSiteName
	}
	if req.IISManagedRuntime != nil {
		updates["iis_managed_runtime"] = normalizeIISManagedRuntime(*req.IISManagedRuntime)
	}
	if req.PublicURL != nil {
		updates["public_url"] = *req.PublicURL
	}
	if req.EnvContent != nil {
		updates["env_content"] = *req.EnvContent
	}

	nextSvc := *svc
	if req.ServiceType != nil {
		nextSvc.ServiceType = normalizeServiceType(*req.ServiceType)
	}
	if req.WindowsServiceName != nil {
		nextSvc.WindowsServiceName = *req.WindowsServiceName
	}
	if req.BinaryName != nil {
		nextSvc.BinaryName = *req.BinaryName
	}
	if req.StartArguments != nil {
		nextSvc.StartArguments = *req.StartArguments
	}
	if req.EnvFile != nil {
		nextSvc.EnvFile = *req.EnvFile
	}
	if req.HealthCheckURL != nil {
		nextSvc.HealthCheckURL = *req.HealthCheckURL
	}
	if req.IISAppKind != nil {
		nextSvc.IISAppKind = *req.IISAppKind
	}
	if req.IISAppPool != nil {
		nextSvc.IISAppPool = *req.IISAppPool
	}
	if req.IISSiteName != nil {
		nextSvc.IISSiteName = *req.IISSiteName
	}
	if req.IISManagedRuntime != nil {
		nextSvc.IISManagedRuntime = *req.IISManagedRuntime
	}
	if req.PublicURL != nil {
		nextSvc.PublicURL = *req.PublicURL
	}
	if req.EnvContent != nil {
		nextSvc.EnvContent = *req.EnvContent
	}
	if err := validateServicePayload(&nextSvc); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	updates["service_type"] = nextSvc.ServiceType
	updates["windows_service_name"] = nextSvc.WindowsServiceName
	updates["binary_name"] = nextSvc.BinaryName
	updates["start_arguments"] = nextSvc.StartArguments
	updates["env_file"] = nextSvc.EnvFile
	updates["health_check_url"] = nextSvc.HealthCheckURL
	updates["iis_app_kind"] = nextSvc.IISAppKind
	updates["iis_app_pool"] = nextSvc.IISAppPool
	updates["iis_site_name"] = nextSvc.IISSiteName
	updates["iis_managed_runtime"] = nextSvc.IISManagedRuntime
	updates["public_url"] = nextSvc.PublicURL
	updates["env_content"] = nextSvc.EnvContent

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
				Target:    normalizeConfigFileTarget(file.Target),
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

	h.db.Preload("ConfigFiles").First(svc, svc.ID)
	c.JSON(http.StatusOK, svc)
}

// DeleteService removes a service.
func (h *Handler) DeleteService(c *gin.Context) {
	svc, err := h.findService(c)
	if err != nil {
		return
	}

	if err := h.cleanupServiceRuntime(c.Request.Context(), svc); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.db.Delete(svc).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.refreshActiveConfigSnapshot(svc.WatcherID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	h.db.Model(&database.Watcher{}).Where("id = ?", svc.WatcherID).UpdateColumn("updated_at", time.Now())
	h.triggerSync()

	c.JSON(http.StatusOK, MessageResponse{Message: "service deleted"})
}

// RedeployWatcher clears the current version and triggers an immediate check to force a fresh deployment.
