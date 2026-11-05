package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// BlockReleaseRequest explains why a version must never be deployed.
type BlockReleaseRequest struct {
	Reason string `json:"reason" binding:"required,max=500"`
}

// BlockRelease blocks a version from every deploy path: polling, approval,
// redeploy, catalog deployment, and manual or automatic rollback.
func (h *Handler) BlockRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	version := strings.TrimSpace(c.Param("version"))
	var req BlockReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if version == "" || reason == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "version and reason are required"})
		return
	}
	mark, err := agent.BlockRelease(h.db, watcher.ID, version, reason)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.log.WithWatcher(watcher.ID, watcher.Name).WithTrace(agent.TraceFromContext(c.Request.Context())).Warn("release blocked", "version", version, "reason", reason)
	c.JSON(http.StatusOK, mark)
}

// UnblockRelease allows a blocked version to be deployed again.
func (h *Handler) UnblockRelease(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	version := strings.TrimSpace(c.Param("version"))
	mark, err := agent.UnblockRelease(h.db, watcher.ID, version)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "release is not marked for this watcher"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	h.log.WithWatcher(watcher.ID, watcher.Name).WithTrace(agent.TraceFromContext(c.Request.Context())).Info("release unblocked", "version", version)
	c.JSON(http.StatusOK, mark)
}

// releaseMarks lists the watcher's release notes and blocks, blocked versions first.
func (h *Handler) releaseMarks(watcherID uint) ([]database.ReleaseMark, error) {
	marks := []database.ReleaseMark{}
	err := h.db.Where("watcher_id = ?", watcherID).Order("blocked_at IS NULL, id DESC").Limit(200).Find(&marks).Error
	return marks, err
}

// releaseBlockedError reports a blocked release as a conflict the operator resolves by unblocking it.
func releaseBlockedError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, agent.ErrReleaseBlocked) {
		status = http.StatusConflict
	}
	c.JSON(status, ErrorResponse{Error: err.Error()})
}
