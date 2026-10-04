package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ensureServiceLogDir(watcherID uint) {
	var w database.Watcher
	if err := h.db.Select("install_dir").First(&w, watcherID).Error; err == nil && w.InstallDir != "" {
		_ = os.MkdirAll(filepath.Join(w.InstallDir, "logs"), 0755)
	}
}

func (h *Handler) StartService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if normalizeServiceType(svc.ServiceType) != "nssm" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dashboard start is only available for NSSM-managed services"})
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	h.ensureServiceLogDir(svc.WatcherID)
	if err := h.serviceManager.Start(c.Request.Context(), svc.WindowsServiceName); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s reached %s", svc.WindowsServiceName, agent.ServiceStateRunning)})
}

func (h *Handler) StopService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if normalizeServiceType(svc.ServiceType) != "nssm" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dashboard stop is only available for NSSM-managed services"})
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	if err := h.serviceManager.Stop(c.Request.Context(), svc.WindowsServiceName); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s reached %s", svc.WindowsServiceName, agent.ServiceStateStopped)})
}

func (h *Handler) RestartService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if normalizeServiceType(svc.ServiceType) != "nssm" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "dashboard restart is only available for NSSM-managed services"})
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	h.ensureServiceLogDir(svc.WatcherID)
	if err := h.serviceManager.Restart(c.Request.Context(), svc.WindowsServiceName); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s restarted and reached %s", svc.WindowsServiceName, agent.ServiceStateRunning)})
}

// ── Health status ─────────────────────────────────────────────────────
