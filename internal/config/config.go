package config

import (
	"fmt"
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

	// LogDir is where watcher writes its own logs
	LogDir string `mapstructure:"LOG_DIR"`

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
}

// LoadConfig reads configuration from a .env file and environment variables.
// Environment variables take precedence over the .env file.
func LoadConfig(envPath string) (*AppConfig, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("LOG_DIR", `D:\apps\watcher\logs`)
	v.SetDefault("NSSM_PATH", `C:\ProgramData\chocolatey\bin\nssm.exe`)
	v.SetDefault("DB_PATH", `watcher.db`)
	v.SetDefault("API_PORT", "8080")
	v.SetDefault("WATCHER_REPO_URL", "https://github.com/fanboykun/watcher")
	v.SetDefault("WATCHER_REPO_URL", "https://github.com/fanboykun/watcher")
	v.SetDefault("ENVIRONMENT", "production")
	v.SetDefault("WATCHER_SERVICE_NAME", "app-watcher")

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

	return &cfg, cfg.validate()
}

func cleanWindowsPath(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")
	return s
}

func (c *AppConfig) validate() error {
	return nil
}
