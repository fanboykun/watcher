package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var appConfigEnvKeys = []string{
	"ENVIRONMENT",
	"GITHUB_TOKEN",
	"GITHUB_DEPLOY_ENABLED",
	"LOG_DIR",
	"LOG_LEVEL",
	"LOG_MAX_SIZE_MB",
	"LOG_MAX_BACKUPS",
	"LOG_MAX_AGE_DAYS",
	"LOG_COMPRESS",
	"NSSM_PATH",
	"DB_PATH",
	"API_PORT",
	"API_BASE_URL",
	"WATCHER_REPO_URL",
	"WATCHER_SERVICE_NAME",
	"WEBHOOK_DEFAULT_URL",
	"WEBHOOK_DEFAULT_SIGNING_SECRET",
	"WEBHOOK_TIMEOUT_SEC",
	"WEBHOOK_RETRY_SCHEDULE_SEC",
	"WEBHOOK_AUTO_PAUSE_ENABLED",
	"WEBHOOK_AUTO_PAUSE_AFTER_FAILURES",
	"WEBHOOK_EVENT_RETENTION_DAYS",
	"WEBHOOK_DELIVERY_RETENTION_DAYS",
	"WEB_ASSETS_PATH",
	"WEB_BASE_PATH",
}

func clearAppConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range appConfigEnvKeys {
		value, exists := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		key, value, exists := key, value, exists
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	clearAppConfigEnvironment(t)

	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Environment != "production" {
		t.Errorf("Environment = %q, want production", cfg.Environment)
	}
	if cfg.LogDir != `D:\apps\watcher\logs` {
		t.Errorf("LogDir = %q", cfg.LogDir)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.LogMaxSizeMB != 100 || cfg.LogMaxBackups != 10 || cfg.LogMaxAgeDays != 30 || !cfg.LogCompress {
		t.Errorf("unexpected log rotation defaults: %+v", cfg)
	}
	if cfg.NssmPath != `C:\ProgramData\chocolatey\bin\nssm.exe` {
		t.Errorf("NssmPath = %q", cfg.NssmPath)
	}
	if cfg.DBPath != "watcher.db" || cfg.APIPort != "8080" {
		t.Errorf("unexpected database/API defaults: DBPath=%q APIPort=%q", cfg.DBPath, cfg.APIPort)
	}
	if !cfg.GitHubDeployEnabled || cfg.WatcherServiceName != "app-watcher" {
		t.Errorf("unexpected deployment defaults: %+v", cfg)
	}
	if cfg.WebhookTimeoutSec != 10 || cfg.WebhookRetryScheduleSec != "0,10,60,300" {
		t.Errorf("unexpected webhook retry defaults: %+v", cfg)
	}
	if !cfg.WebhookAutoPauseEnabled || cfg.WebhookAutoPauseAfter != 5 {
		t.Errorf("unexpected webhook auto-pause defaults: %+v", cfg)
	}
	if cfg.WebhookEventRetentionDays != 90 || cfg.WebhookDeliveryRetentionDays != 30 {
		t.Errorf("unexpected webhook retention defaults: %+v", cfg)
	}
}

func TestLoadConfigFileAndEnvironmentPrecedence(t *testing.T) {
	clearAppConfigEnvironment(t)
	envPath := filepath.Join(t.TempDir(), ".env")
	content := strings.Join([]string{
		"ENVIRONMENT=staging",
		"GITHUB_TOKEN=file-token",
		"GITHUB_DEPLOY_ENABLED=false",
		`LOG_DIR=D:\apps\from-file\logs`,
		"LOG_LEVEL=warn",
		"LOG_MAX_SIZE_MB=25",
		"LOG_MAX_BACKUPS=3",
		"LOG_MAX_AGE_DAYS=7",
		"LOG_COMPRESS=false",
		`NSSM_PATH=C:\tools\nssm.exe`,
		`DB_PATH=D:\data\watcher.db`,
		"API_PORT=9090",
		"API_BASE_URL=https://watcher.example.com",
		"WATCHER_REPO_URL=https://github.com/example/watcher",
		"WATCHER_SERVICE_NAME=watcher-staging",
		"WEBHOOK_DEFAULT_URL=https://hooks.example.com/watcher",
		"WEBHOOK_DEFAULT_SIGNING_SECRET=file-secret",
		"WEBHOOK_TIMEOUT_SEC=20",
		"WEBHOOK_RETRY_SCHEDULE_SEC=0,5,30",
		"WEBHOOK_AUTO_PAUSE_ENABLED=false",
		"WEBHOOK_AUTO_PAUSE_AFTER_FAILURES=8",
		"WEBHOOK_EVENT_RETENTION_DAYS=45",
		"WEBHOOK_DELIVERY_RETENTION_DAYS=15",
	}, "\n") + "\n"
	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		t.Fatalf("write env fixture: %v", err)
	}
	t.Setenv("GITHUB_TOKEN", "environment-token")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("API_PORT", "9191")

	cfg, err := LoadConfig(envPath)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.Environment != "staging" || cfg.GitHubToken != "environment-token" {
		t.Errorf("file/environment precedence failed: %+v", cfg)
	}
	if cfg.LogLevel != "debug" || cfg.APIPort != "9191" {
		t.Errorf("environment overrides not applied: %+v", cfg)
	}
	if cfg.GitHubDeployEnabled || cfg.LogCompress || cfg.WebhookAutoPauseEnabled {
		t.Errorf("false boolean values were not loaded: %+v", cfg)
	}
	if cfg.LogMaxSizeMB != 25 || cfg.LogMaxBackups != 3 || cfg.LogMaxAgeDays != 7 {
		t.Errorf("log rotation values not loaded: %+v", cfg)
	}
	if cfg.LogDir != `D:\apps\from-file\logs` || cfg.NssmPath != `C:\tools\nssm.exe` || cfg.DBPath != `D:\data\watcher.db` {
		t.Errorf("Windows paths not preserved: %+v", cfg)
	}
	if cfg.WebhookDefaultSigningSecret != "file-secret" || cfg.WebhookAutoPauseAfter != 8 {
		t.Errorf("webhook values not loaded: %+v", cfg)
	}
}

func TestLoadConfigReturnsFileReadError(t *testing.T) {
	clearAppConfigEnvironment(t)

	_, err := LoadConfig(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("LoadConfig(directory) error = %v, want read config file error", err)
	}
}

func TestLoadConfigReturnsExplicitMissingFileError(t *testing.T) {
	clearAppConfigEnvironment(t)
	missingPath := filepath.Join(t.TempDir(), "missing.env")

	_, err := LoadConfig(missingPath)
	if err == nil || !strings.Contains(err.Error(), "read config file") {
		t.Fatalf("LoadConfig(missing file) error = %v, want read config file error", err)
	}
}

func TestCleanWindowsPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "unchanged", in: `C:\apps\watcher`, want: `C:\apps\watcher`},
		{name: "newline", in: "C:\new\nservice", want: `C:\new\nservice`},
		{name: "tab", in: "C:\tools\twatcher", want: `C:\tools\twatcher`},
		{name: "all controls", in: "a\rb\bc\fd", want: `a\rb\bc\fd`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanWindowsPath(tt.in); got != tt.want {
				t.Errorf("cleanWindowsPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAppConfigValidate(t *testing.T) {
	valid := AppConfig{LogLevel: "info", LogMaxSizeMB: 1}
	tests := []struct {
		name    string
		mutate  func(*AppConfig)
		wantErr string
	}{
		{name: "valid debug", mutate: func(c *AppConfig) { c.LogLevel = " debug " }},
		{name: "valid warn", mutate: func(c *AppConfig) { c.LogLevel = "WARN" }},
		{name: "invalid level", mutate: func(c *AppConfig) { c.LogLevel = "trace" }, wantErr: "LOG_LEVEL"},
		{name: "non-positive max size", mutate: func(c *AppConfig) { c.LogMaxSizeMB = 0 }, wantErr: "LOG_MAX_SIZE_MB"},
		{name: "negative backups", mutate: func(c *AppConfig) { c.LogMaxBackups = -1 }, wantErr: "LOG_MAX_BACKUPS"},
		{name: "negative age", mutate: func(c *AppConfig) { c.LogMaxAgeDays = -1 }, wantErr: "LOG_MAX_AGE_DAYS"},
		{name: "invalid web assets path with query", mutate: func(c *AppConfig) { c.WebAssetsPath = "/watcher?foo=bar" }, wantErr: "WEB_ASSETS_PATH"},
		{name: "invalid web assets path with spaces", mutate: func(c *AppConfig) { c.WebAssetsPath = "/watcher path" }, wantErr: "WEB_ASSETS_PATH"},
		{name: "valid web assets path", mutate: func(c *AppConfig) { c.WebAssetsPath = "/watcher" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizedWebBasePath(t *testing.T) {
	tests := []struct {
		name       string
		assetsPath string
		basePath   string
		want       string
	}{
		{name: "both empty", assetsPath: "", basePath: "", want: ""},
		{name: "single slash", assetsPath: "/", basePath: "", want: ""},
		{name: "whitespace only", assetsPath: "   ", basePath: "", want: ""},
		{name: "standard subpath", assetsPath: "/watcher", basePath: "", want: "/watcher"},
		{name: "trailing slash trimmed", assetsPath: "/watcher/", basePath: "", want: "/watcher"},
		{name: "leading slash added", assetsPath: "watcher", basePath: "", want: "/watcher"},
		{name: "nested path", assetsPath: "/apps/watcher/", basePath: "", want: "/apps/watcher"},
		{name: "base path fallback", assetsPath: "", basePath: "/watcher-base", want: "/watcher-base"},
		{name: "assets path takes priority over base path", assetsPath: "/watcher-assets", basePath: "/watcher-base", want: "/watcher-assets"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AppConfig{
				WebAssetsPath: tt.assetsPath,
				WebBasePath:   tt.basePath,
			}
			if got := cfg.NormalizedWebBasePath(); got != tt.want {
				t.Errorf("NormalizedWebBasePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWebBasePathRejectsUnsafePrefixes(t *testing.T) {
	for _, path := range []string{`//evil.example`, `/a/../b`, `/a/./b`, `/a//b`, `/a" onclick="alert(1)`, `/a\\b`, `/a%2Fb`, `/a?query`, `/a#fragment`, `/a<b`} {
		t.Run(path, func(t *testing.T) {
			if _, err := NormalizeWebBasePath(path); err == nil {
				t.Fatalf("unsafe prefix accepted: %q", path)
			}
		})
	}
}
