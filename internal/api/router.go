package api

import (
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strings"

	"sync"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/webhook"
	"github.com/fanboykun/watcher/web"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// NewRouter creates a Gin engine with all API routes and embedded SPA.
func NewRouter(db *gorm.DB, nssmPath, logDir, version, githubToken, envPath string, appCfg *config.AppConfig, log *agent.Logger, events *agent.WatcherEventBus, checkTrigger chan uint, syncTrigger chan struct{}, webhookService *webhook.Service, webhookTrigger chan struct{}) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := NewHandler(db, nssmPath, logDir, version, githubToken, envPath, appCfg, log, events, checkTrigger, syncTrigger, webhookService, webhookTrigger)

	registerAPIRoutes(r.Group("/api"), h)

	// ── Serve embedded SPA and dynamic subpath routing for all non-API routes ──
	setupSPA(r, h)

	return r
}

func registerAPIRoutes(apiGroup *gin.RouterGroup, h *Handler) {
	apiGroup.GET("/auth/bootstrap", h.AuthBootstrap)
	apiGroup.POST("/auth/login", h.AuthLogin)
	apiGroup.Use(h.RequireAuth())
	apiGroup.GET("/auth/status", h.AuthStatus)
	apiGroup.PUT("/auth/password", h.UpdateAuthPassword)

	// System
	apiGroup.GET("/status", h.SystemStatus)
	apiGroup.GET("/logs", h.AgentLogs)
	apiGroup.GET("/logs/stream", h.StreamAgentLogs)
	apiGroup.POST("/github/inspect", h.InspectGitHubRepo)

	// ── Services (flat, across all watchers) ──────────────
	services := apiGroup.Group("/services")
	{
		services.GET("", h.ListAllServices)
		services.GET("/:id", h.GetServiceDetail)
		services.POST("/:id/start", h.StartService)
		services.POST("/:id/stop", h.StopService)
		services.POST("/:id/restart", h.RestartService)
		services.PUT("/:id/env", h.SyncServiceEnv)
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
		watchers.GET("/:id/deploys/:did", h.GetDeployLog)
		watchers.GET("/:id/deploys/:did/stream", h.StreamDeployLog)
		watchers.GET("/:id/events", h.StreamWatcherEvents)
		watchers.GET("/:id/polls", h.ListPollEvents)
		watchers.POST("/:id/check", h.TriggerCheck)
		watchers.POST("/:id/redeploy", h.RedeployWatcher)

		// Version management and rollback
		watchers.GET("/:id/versions", h.ListAvailableVersions)
		watchers.POST("/:id/rollback", h.RollbackWatcher)
		watchers.POST("/:id/resume", h.ResumeWatcherUpdates)
		watchers.GET("/:id/webhook-events", h.ListWebhookEvents)
		watchers.GET("/:id/webhook-deliveries", h.ListWebhookDeliveries)
		watchers.GET("/:id/webhook-deliveries/:deliveryId", h.GetWebhookDelivery)
		watchers.POST("/:id/webhook/test", h.SendWatcherWebhookTest)
		watchers.POST("/:id/webhook/resume", h.ResumeWatcherWebhook)
		watchers.DELETE("/:id/versions/:version", h.DeleteWatcherVersion)
	}

	// ── Self-management ──────────────────────────────────
	self := apiGroup.Group("/self")
	{
		self.GET("/version", h.SelfVersion)
		self.GET("/config", h.SelfConfig)
		self.PUT("/config", h.UpdateSelfConfig)
		self.GET("/update-check", h.SelfUpdateCheck)
		self.POST("/update", h.SelfUpdate)
		self.POST("/restart", h.SelfRestart)
		self.POST("/uninstall", h.SelfUninstall)
	}
}

var reSvelteKitBase = regexp.MustCompile(`base:\s*""`)

// processIndexHTML replaces the base path and asset paths in the SvelteKit index.html.
func processIndexHTML(content []byte, basePath string) []byte {
	if basePath == "" {
		return content
	}

	s := string(content)

	// Replace SvelteKit base and assets configuration
	s = reSvelteKitBase.ReplaceAllString(s, fmt.Sprintf(`base: %q, assets: %q`, basePath, basePath))

	// Replace root-relative asset paths for SvelteKit assets and imports
	s = strings.ReplaceAll(s, `href="/_app/`, fmt.Sprintf(`href="%s/_app/`, basePath))
	s = strings.ReplaceAll(s, `src="/_app/`, fmt.Sprintf(`src="%s/_app/`, basePath))
	s = strings.ReplaceAll(s, `import("/_app/`, fmt.Sprintf(`import("%s/_app/`, basePath))
	s = strings.ReplaceAll(s, `import('/_app/`, fmt.Sprintf(`import('%s/_app/`, basePath))

	return []byte(s)
}

// currentWebBasePath returns the active base path prefix for this request.
// If configured in AppConfig, that takes precedence; otherwise it checks the
// X-Forwarded-Prefix header sent by reverse proxies.
func (h *Handler) currentWebBasePath(c *gin.Context) string {
	if h != nil && h.appCfg != nil {
		if p := h.appCfg.NormalizedWebBasePath(); p != "" {
			return p
		}
	}
	if c != nil {
		if p := strings.TrimSpace(c.GetHeader("X-Forwarded-Prefix")); p != "" {
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			return strings.TrimRight(p, "/")
		}
	}
	return ""
}

// setupSPA configures the router to serve the embedded SvelteKit SPA.
// Static assets are served directly; all other paths fall back to index.html
// so SvelteKit's client-side router handles them.
func setupSPA(r *gin.Engine, h *Handler) {
	spaFS, err := web.FS()
	if err != nil {
		// If the embed fails (e.g. dev mode without build), skip SPA serving
		return
	}

	rawIndex, _ := fs.ReadFile(spaFS, "index.html")

	var (
		indexCacheMu sync.RWMutex
		indexCache   = make(map[string][]byte)
	)

	getIndexContent := func(basePath string) []byte {
		if rawIndex == nil {
			return nil
		}
		indexCacheMu.RLock()
		cached, ok := indexCache[basePath]
		indexCacheMu.RUnlock()
		if ok {
			return cached
		}

		transformed := processIndexHTML(rawIndex, basePath)
		indexCacheMu.Lock()
		indexCache[basePath] = transformed
		indexCacheMu.Unlock()
		return transformed
	}

	// Try to serve static files; fall back to index.html for SPA routes
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		basePath := h.currentWebBasePath(c)

		// Handle exact base path redirect: e.g. /watcher -> /watcher/
		if basePath != "" && path == basePath {
			c.Redirect(http.StatusMovedPermanently, basePath+"/")
			return
		}

		// Dynamic API dispatch: if this is an API call under the active base path,
		// strip the base path prefix and re-route to registered API routes.
		if basePath != "" && (strings.HasPrefix(path, basePath+"/api/") || path == basePath+"/api") {
			c.Request.URL.Path = strings.TrimPrefix(path, basePath)
			r.HandleContext(c)
			return
		}

		// When requested with base path prefix, try serving relative asset
		if basePath != "" && strings.HasPrefix(path, basePath+"/") {
			relPath := strings.TrimPrefix(path, basePath+"/")
			if f, err := fs.ReadFile(spaFS, relPath); err == nil {
				c.Data(http.StatusOK, contentType(relPath), f)
				return
			}
		}

		// Try resolving as static asset from root (e.g. /_app/..., /watcher.ico)
		if len(path) > 1 {
			if f, err := fs.ReadFile(spaFS, path[1:]); err == nil {
				c.Data(http.StatusOK, contentType(path), f)
				return
			}
		}

		// SPA fallback: serve index.html for client-side routing
		indexContent := getIndexContent(basePath)
		if indexContent != nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", indexContent)
			return
		}
		c.String(http.StatusNotFound, "not found")
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
