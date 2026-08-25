package api

import (
	"net/http"
	"strconv"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

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
