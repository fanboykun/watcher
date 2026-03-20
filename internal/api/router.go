package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter creates a Gin engine with all API routes registered.
func NewRouter(db *gorm.DB, nssmPath, logDir, version string, checkTrigger chan uint) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := NewHandler(db, nssmPath, logDir, version, checkTrigger)

	api := r.Group("/api")
	{
		// System
		api.GET("/status", h.SystemStatus)
		api.GET("/logs", h.AgentLogs)

		// ── Services (flat, across all watchers) ──────────────
		services := api.Group("/services")
		{
			services.GET("", h.ListAllServices)
			services.GET("/:id", h.GetServiceDetail)
			services.POST("/:id/start", h.StartService)
			services.POST("/:id/stop", h.StopService)
			services.POST("/:id/restart", h.RestartService)
			services.GET("/:id/health", h.GetServiceHealth)
			services.GET("/:id/health/history", h.GetHealthHistory)
			services.GET("/:id/logs", h.GetServiceLogs)
			services.GET("/:id/deploys", h.GetServiceDeploys)
		}

		// ── Watchers ──────────────────────────────────────────
		watchers := api.Group("/watchers")
		{
			watchers.GET("", h.ListWatchers)
			watchers.POST("", h.CreateWatcher)
			watchers.GET("/:id", h.GetWatcher)
			watchers.PUT("/:id", h.UpdateWatcher)
			watchers.DELETE("/:id", h.DeleteWatcher)

			// Nested services under watcher
			watchers.GET("/:id/services", h.ListServices)
			watchers.POST("/:id/services", h.CreateService)
			watchers.PUT("/:id/services/:sid", h.UpdateService)
			watchers.DELETE("/:id/services/:sid", h.DeleteService)

			// Deploy logs and trigger
			watchers.GET("/:id/deploys", h.ListDeployLogs)
			watchers.POST("/:id/check", h.TriggerCheck)
		}
	}

	return r
}
