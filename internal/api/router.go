package api

import (
	"io/fs"
	"net/http"

	"github.com/fanboykun/watcher/web"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter creates a Gin engine with all API routes and embedded SPA.
func NewRouter(db *gorm.DB, nssmPath, logDir, version string, checkTrigger chan uint, syncTrigger chan struct{}) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := NewHandler(db, nssmPath, logDir, version, checkTrigger, syncTrigger)

	apiGroup := r.Group("/api")
	{
		// System
		apiGroup.GET("/status", h.SystemStatus)
		apiGroup.GET("/logs", h.AgentLogs)

		// ── Services (flat, across all watchers) ──────────────
		services := apiGroup.Group("/services")
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
		watchers := apiGroup.Group("/watchers")
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

	// ── Serve embedded SPA for all non-API routes ─────────────
	setupSPA(r)

	return r
}

// setupSPA configures the router to serve the embedded SvelteKit SPA.
// Static assets are served directly; all other paths fall back to index.html
// so SvelteKit's client-side router handles them.
func setupSPA(r *gin.Engine) {
	spaFS, err := web.FS()
	if err != nil {
		// If the embed fails (e.g. dev mode without build), skip SPA serving
		return
	}

	// Try to serve static files; fall back to index.html for SPA routes
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Try to open the file from the embedded FS
		f, err := spaFS.(fs.ReadFileFS).ReadFile(path[1:]) // strip leading "/"
		if err == nil {
			// Serve the file with proper content type
			c.Data(http.StatusOK, contentType(path), f)
			return
		}

		// SPA fallback: serve index.html for client-side routing
		index, err := spaFS.(fs.ReadFileFS).ReadFile("index.html")
		if err != nil {
			c.String(http.StatusNotFound, "not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}

// contentType returns the MIME type based on file extension.
func contentType(path string) string {
	switch {
	case endsWith(path, ".html"):
		return "text/html; charset=utf-8"
	case endsWith(path, ".css"):
		return "text/css; charset=utf-8"
	case endsWith(path, ".js"):
		return "application/javascript"
	case endsWith(path, ".json"):
		return "application/json"
	case endsWith(path, ".svg"):
		return "image/svg+xml"
	case endsWith(path, ".png"):
		return "image/png"
	case endsWith(path, ".ico"):
		return "image/x-icon"
	case endsWith(path, ".woff2"):
		return "font/woff2"
	case endsWith(path, ".woff"):
		return "font/woff"
	default:
		return "application/octet-stream"
	}
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
