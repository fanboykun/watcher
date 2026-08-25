package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func (h *Handler) findWatcher(c *gin.Context) (*database.Watcher, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid watcher id"})
		return nil, err
	}

	var watcher database.Watcher
	if err := h.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "watcher not found"})
		return nil, err
	}
	normalizeWatcherServices(&watcher)
	return &watcher, nil
}

func (h *Handler) findService(c *gin.Context) (*database.Service, error) {
	// Verify watcher exists
	watcher, err := h.findWatcher(c)
	if err != nil {
		return nil, err
	}

	sid, err := strconv.ParseUint(c.Param("sid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := h.db.Preload("ConfigFiles").Where("id = ? AND watcher_id = ?", sid, watcher.ID).First(&svc).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	normalizeServiceCollections(&svc)
	return &svc, nil
}

func withDefault(val, def int) int {
	if val <= 0 {
		return def
	}
	return val
}

func compareSemver(a, b string) int {
	cmp, ok := agent.CompareVersions(a, b)
	if !ok {
		parse := func(v string) [3]int {
			var out [3]int
			v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
			parts := strings.Split(v, ".")
			for i := 0; i < 3 && i < len(parts); i++ {
				fmt.Sscanf(parts[i], "%d", &out[i])
			}
			return out
		}

		pa := parse(a)
		pb := parse(b)
		for i := 0; i < 3; i++ {
			if pa[i] > pb[i] {
				return 1
			}
			if pa[i] < pb[i] {
				return -1
			}
		}
		return 0
	}
	return cmp
}

func buildWatcherLogURL(apiBaseURL string, watcherID, deployLogID uint) string {
	base := normalizeUIBaseURL(apiBaseURL)
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s/watchers/%d/logs/%d", base, watcherID, deployLogID)
}

func normalizeUIBaseURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	if strings.EqualFold(u.Path, "/api") {
		u.Path = ""
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/")
}

func enrichWatcherSecrets(w *database.Watcher) {
	if w == nil {
		return
	}
	token := strings.TrimSpace(w.GitHubToken)
	w.HasGitHubToken = token != ""
	w.GitHubTokenMasked = maskToken(token)
	webhookSecret := strings.TrimSpace(w.WebhookSigningSecret)
	w.HasWebhookSigningSecret = webhookSecret != ""
	w.WebhookSigningSecretMasked = maskToken(webhookSecret)
}

func validateResolvedWebhookConfig(enabled bool, watcherURL, watcherSecret string, cfg *config.AppConfig) error {
	if !enabled {
		return nil
	}

	resolvedURL := strings.TrimSpace(watcherURL)
	if resolvedURL == "" && cfg != nil {
		resolvedURL = strings.TrimSpace(cfg.WebhookDefaultURL)
	}
	if resolvedURL == "" {
		return fmt.Errorf("webhook_url is required when webhook delivery is enabled")
	}

	resolvedSecret := strings.TrimSpace(watcherSecret)
	if resolvedSecret == "" && cfg != nil {
		resolvedSecret = strings.TrimSpace(cfg.WebhookDefaultSigningSecret)
	}
	if resolvedSecret == "" {
		return fmt.Errorf("a webhook signing secret is required when webhook delivery is enabled")
	}
	return validateOptionalWebhookSigningSecret(resolvedSecret)
}

// timeNow returns a pointer to the current UTC time.
func timeNow() *time.Time {
	t := time.Now().UTC()
	return &t
}

func (h *Handler) triggerSync() {
	select {
	case h.syncTrigger <- struct{}{}:
	default:
	}
}

// touchWatchersUpdatedAt forces the agent to recreate repo watchers on next sync.
func (h *Handler) touchWatchersUpdatedAt() {
	h.db.Model(&database.Watcher{}).Where("1 = 1").UpdateColumn("updated_at", time.Now())
}
