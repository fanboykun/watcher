package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *Handler) RedeployWatcher(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if watcher.PendingCatalogID != 0 {
		catalogError(c, errCatalogConflict)
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

	now := time.Now().UTC()
	queuedVersion := strings.TrimSpace(watcher.CurrentVersion)
	if queuedVersion == "" {
		queuedVersion = "pending"
	}
	dlog := database.DeployLog{
		WatcherID:   watcher.ID,
		TriggeredBy: "manual",
		Kind:        "deploy",
		Reason:      "manual_redeploy",
		Version:     queuedVersion,
		FromVersion: watcher.CurrentVersion,
		Status:      "in_progress",
		StartedAt:   &now,
		Logs:        "redeploy: queued manual redeploy request",
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var live database.Watcher
		if err := tx.First(&live, watcher.ID).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&database.DeployLog{}).Where("watcher_id = ? AND completed_at IS NULL", watcher.ID).Count(&active).Error; err != nil {
			return err
		}
		if live.PendingCatalogID != 0 || active != 0 || live.Status == "deploying" {
			return errCatalogConflict
		}
		// Reserve the attempt and force the next poll only after all operation guards pass.
		if err := tx.Model(&live).UpdateColumns(map[string]any{"current_version": "", "last_error": ""}).Error; err != nil {
			return err
		}
		return tx.Create(&dlog).Error
	}); err != nil {
		catalogError(c, err)
		return
	}

	_ = h.db.Model(&dlog).Update("root_attempt_id", dlog.ID).Error
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

	// Runtime reads versions from the DB; queue without restarting an uncorrelated cycle.
	// Trigger immediate check
	_, _ = h.queueCheck(c, watcher.ID)

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
