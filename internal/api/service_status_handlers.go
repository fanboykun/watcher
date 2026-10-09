package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetServiceStatus probes runtime and health in parallel and saves both observations.
func (h *Handler) GetServiceStatus(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	var watcher database.Watcher
	if err := h.db.First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "parent watcher not found"})
		return
	}

	type runtimeResult struct{ status, error string }
	runtimeDone := make(chan runtimeResult, 1)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	go func() {
		status, statusError := h.probeServiceRuntime(ctx, svc)
		runtimeDone <- runtimeResult{status, statusError}
	}()
	event, _ := h.probeServiceHealth(ctx, svc, &watcher)
	runtime := <-runtimeDone
	checkedAt := timeNow()

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.Service{}).Where("id = ?", svc.ID).UpdateColumns(map[string]any{
			"last_service_status":     runtime.status,
			"last_service_error":      runtime.error,
			"last_service_checked_at": checkedAt,
			"last_health_status":      event.Status,
			"last_health_http_status": event.HTTPStatus,
			"last_health_error":       event.Error,
			"last_health_checked_at":  event.CheckedAt,
		}).Error; err != nil {
			return err
		}
		if h.webhooks != nil {
			return h.webhooks.EmitHealthChangedTx(tx, &watcher, svc, &event)
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	svc.LastServiceStatus, svc.LastServiceError, svc.LastServiceCheckedAt = runtime.status, runtime.error, checkedAt
	svc.LastHealthStatus, svc.LastHealthHTTPStatus = event.Status, event.HTTPStatus
	svc.LastHealthError, svc.LastHealthCheckedAt = event.Error, event.CheckedAt
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, svc)
}

func (h *Handler) probeServiceRuntime(ctx context.Context, svc *database.Service) (string, string) {
	if normalizeServiceType(svc.ServiceType) != "nssm" {
		return "not_applicable", ""
	}
	if !h.runningOnWindows() || h.serviceManager == nil {
		return "unknown", "NSSM status is only available on Windows with NSSM configured"
	}
	state, err := h.serviceManager.Status(ctx, svc.WindowsServiceName)
	if errors.Is(err, agent.ErrServiceNotFound) {
		return "not_installed", err.Error()
	}
	if err != nil {
		return "unknown", err.Error()
	}
	if state == "" {
		return "unknown", "NSSM returned no service state"
	}
	return strings.ToLower(strings.TrimPrefix(string(state), "SERVICE_")), ""
}
