package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) GetServiceHealth(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	var watcher database.Watcher
	if err := h.db.First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "parent watcher not found"})
		return
	}
	event, healthURL := h.probeServiceHealth(c.Request.Context(), svc, &watcher)

	// Record the event and refresh last-known state.
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.Service{}).Where("id = ?", svc.ID).UpdateColumns(map[string]any{
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

	c.JSON(http.StatusOK, gin.H{
		"service_id":   svc.ID,
		"service_name": svc.WindowsServiceName,
		"health_url":   healthURL,
		"status":       event.Status,
		"http_status":  event.HTTPStatus,
		"error":        event.Error,
		"checked_at":   event.CheckedAt,
	})
}

// probeServiceHealth performs one bounded request, independently of runtime status.
func (h *Handler) probeServiceHealth(ctx context.Context, svc *database.Service, watcher *database.Watcher) (database.HealthEvent, string) {
	event := database.HealthEvent{
		ServiceID: svc.ID, CheckedAt: timeNow(), Source: "manual",
		PreviousStatus: svc.LastHealthStatus, Status: "unknown",
	}
	healthURL := svc.HealthCheckURL
	if healthURL == "" {
		healthURL = watcher.HcURL
	}
	if healthURL == "" {
		return event, healthURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		event.Status, event.Error = "error", err.Error()
		return event, healthURL
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if h.log != nil {
		client.Transport = agent.NewTraceTransport(nil, h.log.WithWatcher(watcher.ID, watcher.Name))
	}
	resp, err := client.Do(req)
	if err != nil {
		event.Status, event.Error = "error", err.Error()
		return event, healthURL
	}
	defer resp.Body.Close()
	event.HTTPStatus = resp.StatusCode
	event.Status = "unhealthy"
	if resp.StatusCode == http.StatusOK {
		event.Status = "healthy"
	}
	return event, healthURL
}

func (h *Handler) GetHealthHistory(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	var events []database.HealthEvent
	if err := h.db.Where("service_id = ?", svc.ID).
		Order("id desc").Limit(limit).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

// ── Service logs ──────────────────────────────────────────────────────
