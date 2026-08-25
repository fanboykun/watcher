package api

import (
	"net/http"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) ListAllServices(c *gin.Context) {
	services := make([]database.Service, 0)
	if err := h.db.Preload("ConfigFiles").Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	type serviceWithWatcher struct {
		database.Service
		WatcherName string `json:"watcher_name"`
		InstallDir  string `json:"install_dir"`
	}

	services = normalizeServices(services)
	result := make([]serviceWithWatcher, 0, len(services))
	for _, svc := range services {
		var watcher database.Watcher
		h.db.Select("name", "install_dir").First(&watcher, svc.WatcherID)
		result = append(result, serviceWithWatcher{
			Service:     svc,
			WatcherName: watcher.Name,
			InstallDir:  watcher.InstallDir,
		})
	}
	c.JSON(http.StatusOK, result)
}

// ── Service start/stop/restart ────────────────────────────────────────
