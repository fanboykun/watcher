package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListWebhookEvents(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var events []database.WebhookEvent
	if err := h.db.Where("watcher_id = ?", watcher.ID).Order("occurred_at desc, id desc").Limit(200).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

func (h *Handler) ListWebhookDeliveries(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := h.db.Model(&database.WebhookDelivery{}).Where("watcher_id = ?", watcher.ID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	var deliveries []database.WebhookDelivery
	if err := query.Order("created_at desc, id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&deliveries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"data":     deliveries,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

func (h *Handler) GetWebhookDelivery(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	id, err := strconv.ParseUint(c.Param("deliveryId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid delivery id"})
		return
	}

	var delivery database.WebhookDelivery
	if err := h.db.Where("id = ? AND watcher_id = ?", id, watcher.ID).First(&delivery).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "webhook delivery not found"})
		return
	}

	var event database.WebhookEvent
	_ = h.db.First(&event, delivery.WebhookEventID).Error
	var payload any
	if strings.TrimSpace(event.Payload) != "" {
		_ = json.Unmarshal([]byte(event.Payload), &payload)
	}

	c.JSON(http.StatusOK, gin.H{
		"delivery": delivery,
		"event": gin.H{
			"id":             event.ID,
			"event_id":       event.EventID,
			"event_type":     event.EventType,
			"schema_version": event.SchemaVersion,
			"status":         event.Status,
			"summary":        event.Summary,
			"occurred_at":    event.OccurredAt,
			"payload":        payload,
		},
		"request": gin.H{
			"url":          delivery.ResolvedURL,
			"auth_type":    delivery.AuthType,
			"token_source": delivery.TokenSource,
			"headers": gin.H{
				"content_type":          "application/json",
				"x_watcher_event":       event.EventType,
				"x_watcher_delivery_id": delivery.DeliveryID,
			},
		},
	})
}

func (h *Handler) SendWatcherWebhookTest(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	if h.webhooks == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "webhook service unavailable"})
		return
	}
	if err := h.webhooks.EmitTestEvent(watcher); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, MessageResponse{Message: "webhook test queued"})
}

func (h *Handler) ResumeWatcherWebhook(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	var req ResumeWebhookRequest
	_ = c.ShouldBindJSON(&req)
	updates := map[string]any{
		"webhook_paused_at":      nil,
		"webhook_pause_reason":   "",
		"webhook_failure_streak": 0,
	}
	if err := h.db.Model(watcher).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if req.ReplaySuppressed {
		now := time.Now().UTC()
		if err := h.db.Model(&database.WebhookEvent{}).
			Where("watcher_id = ? AND status = ?", watcher.ID, "suppressed").
			Updates(map[string]any{"status": "pending", "suppressed_at": nil, "updated_at": &now}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	if h.webhooks != nil {
		h.webhooks.Nudge()
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook delivery resumed", "replay_suppressed": req.ReplaySuppressed})
}
