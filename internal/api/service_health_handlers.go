package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) GetServiceHealth(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	if svc.HealthCheckURL == "" {
		c.JSON(http.StatusOK, gin.H{
			"service_id":   svc.ID,
			"service_name": svc.WindowsServiceName,
			"status":       "unknown",
			"message":      "no health check URL configured",
		})
		return
	}

	// Perform live health check
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(svc.HealthCheckURL)

	event := database.HealthEvent{
		ServiceID:      svc.ID,
		CheckedAt:      timeNow(),
		Source:         "manual",
		PreviousStatus: svc.LastHealthStatus,
	}

	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
	} else {
		event.HTTPStatus = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode == 200 {
			event.Status = "healthy"
		} else {
			event.Status = "unhealthy"
		}
	}

	var watcher database.Watcher
	if err := h.db.First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "parent watcher not found"})
		return
	}

	// Record the event and refresh last-known state.
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.Service{}).Where("id = ?", svc.ID).Updates(map[string]any{
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
		"health_url":   svc.HealthCheckURL,
		"status":       event.Status,
		"http_status":  event.HTTPStatus,
		"error":        event.Error,
		"checked_at":   event.CheckedAt,
	})
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
