package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

// GetWatcherConfigSnapshot returns only captured historical files. It never
// substitutes active configuration when a version or category is missing.
func (h *Handler) GetWatcherConfigSnapshot(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	snapshot, err := agent.ReadConfigSnapshot(watcher.InstallDir, c.Param("version"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, agent.ErrConfigSnapshotNotFound) || os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		c.JSON(status, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, snapshot)
}
