package api

import (
	"net/http"
	"strconv"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) GetServiceDeploys(c *gin.Context) {
	svc, err := h.findServiceByID(c)
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

	// Deploy logs are per-watcher, return paginated history for this service's watcher.
	query := h.db.Model(&database.DeployLog{}).Where("watcher_id = ?", svc.WatcherID)
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

// ── Helpers ───────────────────────────────────────────────────────────
