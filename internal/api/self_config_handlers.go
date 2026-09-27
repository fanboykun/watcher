package api

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/gin-gonic/gin"
)

func (h *Handler) SelfVersion(c *gin.Context) {
	exePath, _ := os.Executable()
	c.JSON(http.StatusOK, gin.H{
		"version":    h.version,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"executable": exePath,
	})
}

// SelfConfig returns the current agent configuration loaded from .env.
func (h *Handler) SelfConfig(c *gin.Context) {
	if h.appCfg == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "app config is not available"})
		return
	}

	c.JSON(http.StatusOK, SelfConfigResponse{
		Environment:                       h.appCfg.Environment,
		GitHubDeployEnabled:               h.appCfg.GitHubDeployEnabled,
		LogDir:                            h.appCfg.LogDir,
		LogLevel:                          h.appCfg.LogLevel,
		LogMaxSizeMB:                      h.appCfg.LogMaxSizeMB,
		LogMaxBackups:                     h.appCfg.LogMaxBackups,
		LogMaxAgeDays:                     h.appCfg.LogMaxAgeDays,
		LogCompress:                       h.appCfg.LogCompress,
		NssmPath:                          h.appCfg.NssmPath,
		DBPath:                            h.appCfg.DBPath,
		APIPort:                           h.appCfg.APIPort,
		APIBaseURL:                        h.appCfg.APIBaseURL,
		WatcherRepoURL:                    h.appCfg.WatcherRepoURL,
		WatcherServiceName:                h.selfServiceName(),
		HasGitHubToken:                    strings.TrimSpace(h.appCfg.GitHubToken) != "",
		GitHubTokenMasked:                 maskToken(h.appCfg.GitHubToken),
		WebhookDefaultURL:                 h.appCfg.WebhookDefaultURL,
		HasWebhookDefaultSigningSecret:    strings.TrimSpace(h.appCfg.WebhookDefaultSigningSecret) != "",
		WebhookDefaultSigningSecretMasked: maskToken(h.appCfg.WebhookDefaultSigningSecret),
		WebhookTimeoutSec:                 h.appCfg.WebhookTimeoutSec,
		WebhookRetryScheduleSec:           h.appCfg.WebhookRetryScheduleSec,
		WebhookAutoPauseEnabled:           h.appCfg.WebhookAutoPauseEnabled,
		WebhookAutoPauseAfterFailures:     h.appCfg.WebhookAutoPauseAfter,
		WebhookEventRetentionDays:         h.appCfg.WebhookEventRetentionDays,
		WebhookDeliveryRetentionDays:      h.appCfg.WebhookDeliveryRetentionDays,
		WebAssetsPath:                     h.appCfg.WebAssetsPath,
		EnvPath:                           h.envPath,
	})
}

// UpdateSelfConfig updates selected agent config values and persists them to .env.
// For fields used by running watcher loops, the agent goroutines are recreated.
func (h *Handler) UpdateSelfConfig(c *gin.Context) {
	if h.appCfg == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "app config is not available"})
		return
	}
	var req UpdateSelfConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	next := *h.appCfg
	if req.Environment != nil {
		next.Environment = strings.TrimSpace(*req.Environment)
	}
	if req.GitHubToken != nil {
		next.GitHubToken = strings.TrimSpace(*req.GitHubToken)
	}
	if req.GitHubDeployEnabled != nil {
		next.GitHubDeployEnabled = *req.GitHubDeployEnabled
	}
	if req.LogDir != nil {
		next.LogDir = strings.TrimSpace(*req.LogDir)
	}
	if req.LogLevel != nil {
		next.LogLevel = strings.TrimSpace(*req.LogLevel)
	}
	if req.LogMaxSizeMB != nil {
		next.LogMaxSizeMB = *req.LogMaxSizeMB
	}
	if req.LogMaxBackups != nil {
		next.LogMaxBackups = *req.LogMaxBackups
	}
	if req.LogMaxAgeDays != nil {
		next.LogMaxAgeDays = *req.LogMaxAgeDays
	}
	if req.LogCompress != nil {
		next.LogCompress = *req.LogCompress
	}
	if err := next.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if req.NssmPath != nil {
		next.NssmPath = strings.TrimSpace(*req.NssmPath)
	}
	if req.DBPath != nil {
		next.DBPath = strings.TrimSpace(*req.DBPath)
	}
	if req.APIPort != nil {
		port := strings.TrimSpace(*req.APIPort)
		if err := validatePort(port); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		next.APIPort = port
	}
	if req.APIBaseURL != nil {
		next.APIBaseURL = strings.TrimSpace(*req.APIBaseURL)
	}
	if req.WatcherRepoURL != nil {
		next.WatcherRepoURL = strings.TrimSpace(*req.WatcherRepoURL)
	}
	if req.WatcherServiceName != nil {
		next.WatcherServiceName = strings.TrimSpace(*req.WatcherServiceName)
	}
	if req.WebhookDefaultURL != nil {
		next.WebhookDefaultURL = strings.TrimSpace(*req.WebhookDefaultURL)
	}
	if req.WebhookDefaultSigningSecret != nil {
		secret := strings.TrimSpace(*req.WebhookDefaultSigningSecret)
		if err := validateOptionalWebhookSigningSecret(secret); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		next.WebhookDefaultSigningSecret = secret
	}
	if req.WebhookTimeoutSec != nil {
		next.WebhookTimeoutSec = *req.WebhookTimeoutSec
	}
	if req.WebhookRetryScheduleSec != nil {
		next.WebhookRetryScheduleSec = strings.TrimSpace(*req.WebhookRetryScheduleSec)
	}
	if req.WebhookAutoPauseEnabled != nil {
		next.WebhookAutoPauseEnabled = *req.WebhookAutoPauseEnabled
	}
	if req.WebhookAutoPauseAfterFailures != nil {
		next.WebhookAutoPauseAfter = *req.WebhookAutoPauseAfterFailures
	}
	if req.WebhookEventRetentionDays != nil {
		next.WebhookEventRetentionDays = *req.WebhookEventRetentionDays
	}
	if req.WebhookDeliveryRetentionDays != nil {
		next.WebhookDeliveryRetentionDays = *req.WebhookDeliveryRetentionDays
	}
	if req.WebAssetsPath != nil {
		next.WebAssetsPath = strings.TrimSpace(*req.WebAssetsPath)
	}
	if err := h.validateWebhookDefaultsDependency(&next); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	updates := map[string]string{
		"ENVIRONMENT":                       next.Environment,
		"GITHUB_TOKEN":                      next.GitHubToken,
		"GITHUB_DEPLOY_ENABLED":             strconv.FormatBool(next.GitHubDeployEnabled),
		"WEBHOOK_DEFAULT_URL":               next.WebhookDefaultURL,
		"WEBHOOK_DEFAULT_SIGNING_SECRET":    next.WebhookDefaultSigningSecret,
		"WEBHOOK_TIMEOUT_SEC":               strconv.Itoa(next.WebhookTimeoutSec),
		"WEBHOOK_RETRY_SCHEDULE_SEC":        next.WebhookRetryScheduleSec,
		"WEBHOOK_AUTO_PAUSE_ENABLED":        strconv.FormatBool(next.WebhookAutoPauseEnabled),
		"WEBHOOK_AUTO_PAUSE_AFTER_FAILURES": strconv.Itoa(next.WebhookAutoPauseAfter),
		"WEBHOOK_EVENT_RETENTION_DAYS":      strconv.Itoa(next.WebhookEventRetentionDays),
		"WEBHOOK_DELIVERY_RETENTION_DAYS":   strconv.Itoa(next.WebhookDeliveryRetentionDays),
		"WEB_ASSETS_PATH":                   next.WebAssetsPath,
		"LOG_DIR":                           next.LogDir,
		"LOG_LEVEL":                         next.LogLevel,
		"LOG_MAX_SIZE_MB":                   strconv.Itoa(next.LogMaxSizeMB),
		"LOG_MAX_BACKUPS":                   strconv.Itoa(next.LogMaxBackups),
		"LOG_MAX_AGE_DAYS":                  strconv.Itoa(next.LogMaxAgeDays),
		"LOG_COMPRESS":                      strconv.FormatBool(next.LogCompress),
		"NSSM_PATH":                         next.NssmPath,
		"DB_PATH":                           next.DBPath,
		"API_PORT":                          next.APIPort,
		"API_BASE_URL":                      next.APIBaseURL,
		"WATCHER_REPO_URL":                  next.WatcherRepoURL,
		"WATCHER_SERVICE_NAME":              next.WatcherServiceName,
	}
	if err := config.UpdateEnvFile(h.envPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	// Apply changes to in-memory runtime config.
	*h.appCfg = next
	h.githubToken = next.GitHubToken
	h.nssmPath = next.NssmPath
	h.serviceManager = agent.NewNSSMServiceManager(next.NssmPath)
	h.logDir = next.LogDir

	// Recreate watcher goroutines so they pick up updated global values.
	h.touchWatchersUpdatedAt()
	h.triggerSync()

	c.JSON(http.StatusOK, gin.H{
		"message": "agent configuration saved",
		"notes": []string{
			"watcher loops were reloaded to apply runtime fields",
			"API_PORT, DB_PATH, and logging changes require a manual service restart to fully take effect",
		},
		"config": SelfConfigResponse{
			Environment:                       next.Environment,
			GitHubDeployEnabled:               next.GitHubDeployEnabled,
			LogDir:                            next.LogDir,
			LogLevel:                          next.LogLevel,
			LogMaxSizeMB:                      next.LogMaxSizeMB,
			LogMaxBackups:                     next.LogMaxBackups,
			LogMaxAgeDays:                     next.LogMaxAgeDays,
			LogCompress:                       next.LogCompress,
			NssmPath:                          next.NssmPath,
			DBPath:                            next.DBPath,
			APIPort:                           next.APIPort,
			APIBaseURL:                        next.APIBaseURL,
			WatcherRepoURL:                    next.WatcherRepoURL,
			WatcherServiceName:                h.selfServiceName(),
			HasGitHubToken:                    strings.TrimSpace(next.GitHubToken) != "",
			GitHubTokenMasked:                 maskToken(next.GitHubToken),
			WebhookDefaultURL:                 next.WebhookDefaultURL,
			HasWebhookDefaultSigningSecret:    strings.TrimSpace(next.WebhookDefaultSigningSecret) != "",
			WebhookDefaultSigningSecretMasked: maskToken(next.WebhookDefaultSigningSecret),
			WebhookTimeoutSec:                 next.WebhookTimeoutSec,
			WebhookRetryScheduleSec:           next.WebhookRetryScheduleSec,
			WebhookAutoPauseEnabled:           next.WebhookAutoPauseEnabled,
			WebhookAutoPauseAfterFailures:     next.WebhookAutoPauseAfter,
			WebhookEventRetentionDays:         next.WebhookEventRetentionDays,
			WebhookDeliveryRetentionDays:      next.WebhookDeliveryRetentionDays,
			WebAssetsPath:                     next.WebAssetsPath,
			EnvPath:                           h.envPath,
		},
	})
}

// SelfUpdateCheck checks for a newer version of the watcher from its GitHub repo.
