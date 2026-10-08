package config

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds the environment-level settings loaded from .env.
// Watcher-specific config now lives in the database.
type AppConfig struct {
	// Environment is a human-readable label for this server (informational)
	Environment string `mapstructure:"ENVIRONMENT"`

	// GitHubToken is a PAT with repo scope, shared across all watched repos
	GitHubToken string `mapstructure:"GITHUB_TOKEN"`

	// GitHubDeployEnabled toggles GitHub Deployment API integration.
	// Defaults to true.
	GitHubDeployEnabled bool `mapstructure:"GITHUB_DEPLOY_ENABLED"`

	// LogDir is where watcher writes its own logs
	LogDir string `mapstructure:"LOG_DIR"`

	// LogLevel controls the minimum structured log level: debug, info, warn, or error.
	LogLevel string `mapstructure:"LOG_LEVEL"`

	// Log rotation is handled by lumberjack. Sizes are in megabytes and ages in days.
	LogMaxSizeMB  int  `mapstructure:"LOG_MAX_SIZE_MB"`
	LogMaxBackups int  `mapstructure:"LOG_MAX_BACKUPS"`
	LogMaxAgeDays int  `mapstructure:"LOG_MAX_AGE_DAYS"`
	LogCompress   bool `mapstructure:"LOG_COMPRESS"`

	// NssmPath is the full path to nssm.exe
	NssmPath string `mapstructure:"NSSM_PATH"`

	// DBPath is the path to the SQLite database file
	DBPath string `mapstructure:"DB_PATH"`

	// APIPort is the port for the REST API server
	APIPort string `mapstructure:"API_PORT"`

	// APIBaseURL is the externally reachable base URL for this watcher instance.
	// Used to construct deploy log UI URLs for GitHub Deployment API.
	// Example: "http://192.168.1.100:8080"
	// If empty, GitHub Deployment API integration is disabled.
	APIBaseURL string `mapstructure:"API_BASE_URL"`

	// WatcherRepoURL is the GitHub repository URL for the watcher project itself.
	// Used for self-update checks.
	WatcherRepoURL string `mapstructure:"WATCHER_REPO_URL"`

	// WatcherServiceName is the NSSM service name for the watcher itself.
	// Used by self-update/restart/uninstall actions.
	WatcherServiceName string `mapstructure:"WATCHER_SERVICE_NAME"`

	// Webhook transport defaults.
	WebhookDefaultURL            string `mapstructure:"WEBHOOK_DEFAULT_URL"`
	WebhookDefaultSigningSecret  string `mapstructure:"WEBHOOK_DEFAULT_SIGNING_SECRET"`
	WebhookTimeoutSec            int    `mapstructure:"WEBHOOK_TIMEOUT_SEC"`
	WebhookRetryScheduleSec      string `mapstructure:"WEBHOOK_RETRY_SCHEDULE_SEC"`
	WebhookAutoPauseEnabled      bool   `mapstructure:"WEBHOOK_AUTO_PAUSE_ENABLED"`
	WebhookAutoPauseAfter        int    `mapstructure:"WEBHOOK_AUTO_PAUSE_AFTER_FAILURES"`
	WebhookEventRetentionDays    int    `mapstructure:"WEBHOOK_EVENT_RETENTION_DAYS"`
	WebhookDeliveryRetentionDays int    `mapstructure:"WEBHOOK_DELIVERY_RETENTION_DAYS"`

	// WebAssetsPath is the base path where web assets and dashboard routes are served.
	// Useful when running behind a reverse proxy subpath (e.g. "/watcher").
	// Example: "/watcher"
	WebAssetsPath string `mapstructure:"WEB_ASSETS_PATH"`

	// WebBasePath is an alias for WebAssetsPath.
	WebBasePath string `mapstructure:"WEB_BASE_PATH"`
}

// LoadConfig reads configuration from a .env file and environment variables.
// Environment variables take precedence over the .env file.
func LoadConfig(envPath string) (*AppConfig, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("LOG_DIR", `D:\apps\watcher\logs`)
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("LOG_MAX_SIZE_MB", 100)
	v.SetDefault("LOG_MAX_BACKUPS", 10)
	v.SetDefault("LOG_MAX_AGE_DAYS", 30)
	v.SetDefault("LOG_COMPRESS", true)
	v.SetDefault("NSSM_PATH", `C:\ProgramData\chocolatey\bin\nssm.exe`)
	v.SetDefault("DB_PATH", `watcher.db`)
	v.SetDefault("API_PORT", "8080")
	v.SetDefault("WATCHER_REPO_URL", "https://github.com/fanboykun/watcher")
	v.SetDefault("WATCHER_REPO_URL", "https://github.com/fanboykun/watcher")
	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("GITHUB_DEPLOY_ENABLED", true)
	v.SetDefault("WATCHER_SERVICE_NAME", "app-watcher")
	v.SetDefault("WEBHOOK_TIMEOUT_SEC", 10)
	v.SetDefault("WEBHOOK_RETRY_SCHEDULE_SEC", "0,10,60,300")
	v.SetDefault("WEBHOOK_AUTO_PAUSE_ENABLED", true)
	v.SetDefault("WEBHOOK_AUTO_PAUSE_AFTER_FAILURES", 5)
	v.SetDefault("WEBHOOK_EVENT_RETENTION_DAYS", 90)
	v.SetDefault("WEBHOOK_DELIVERY_RETENTION_DAYS", 30)
	v.SetDefault("WEB_ASSETS_PATH", "")
	v.SetDefault("WEB_BASE_PATH", "")

	// Read .env file
	if envPath != "" {
		v.SetConfigFile(envPath)
		v.SetConfigType("env")
		if err := v.ReadInConfig(); err != nil {
			// Only error if the file was explicitly specified and not found
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("read config file %q: %w", envPath, err)
			}
		}
	}

	// Environment variables override .env values
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Fix viper/gotenv unescaping Windows paths containing \n, \t, etc.
	cfg.NssmPath = cleanWindowsPath(cfg.NssmPath)
	cfg.LogDir = cleanWindowsPath(cfg.LogDir)
	cfg.DBPath = cleanWindowsPath(cfg.DBPath)

	return &cfg, cfg.Validate()
}

func cleanWindowsPath(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")
	return s
}

// Validate checks configuration values that can be changed through the API.
func (c *AppConfig) Validate() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.ToUpper(strings.TrimSpace(c.LogLevel)))); err != nil {
		return fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error: %w", err)
	}
	if c.LogMaxSizeMB <= 0 {
		return fmt.Errorf("LOG_MAX_SIZE_MB must be greater than zero")
	}
	if c.LogMaxBackups < 0 {
		return fmt.Errorf("LOG_MAX_BACKUPS cannot be negative")
	}
	if c.LogMaxAgeDays < 0 {
		return fmt.Errorf("LOG_MAX_AGE_DAYS cannot be negative")
	}
	if _, err := NormalizeWebBasePath(c.webBasePath()); err != nil {
		return err
	}
	return nil
}

// NormalizedWebBasePath returns the normalized base path for the web dashboard and assets.
// It trims whitespace, ensures a single leading slash, removes trailing slashes,
// and returns an empty string if set to "" or "/".
func (c *AppConfig) NormalizedWebBasePath() string {
	path, _ := NormalizeWebBasePath(c.webBasePath())
	return path
}

func (c *AppConfig) webBasePath() string {
	if strings.TrimSpace(c.WebAssetsPath) != "" {
		return c.WebAssetsPath
	}
	return c.WebBasePath
}

var webPathSegment = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// NormalizeWebBasePath also validates proxy-supplied prefixes before embedding
// them in HTML or using them to dispatch API requests.
func NormalizeWebBasePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return "", nil
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	raw = strings.TrimRight(raw, "/")
	for _, segment := range strings.Split(strings.TrimPrefix(raw, "/"), "/") {
		if !webPathSegment.MatchString(segment) || segment == "." || segment == ".." {
			return "", fmt.Errorf("WEB_ASSETS_PATH must be a URL path prefix with safe non-empty segments")
		}
	}
	return raw, nil
}
