package internal

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is loaded from config.json on the server.
// Each server has its own config.json describing which services to manage.
type Config struct {
	// ServiceName must match the key inside version.json published by GitHub Actions
	// e.g. "admin-be"
	ServiceName string `json:"service_name"`

	// Environment is a human-readable label for this server (informational only)
	// e.g. "production", "estate-1"
	Environment string `json:"environment"`

	// InstallDir is the base directory for all releases on this machine
	// e.g. "C:\\apps\\admin-be"
	InstallDir string `json:"install_dir"`

	// CheckIntervalSec is how often (in seconds) to poll for a new version
	CheckIntervalSec int `json:"check_interval_sec"`

	// MetadataURL is the URL to version.json published on the GitHub Release
	// e.g. "https://github.com/your-org/your-repo/releases/latest/download/version.json"
	MetadataURL string `json:"metadata_url"`

	// GitHubToken is a PAT with read:packages or repo scope for private repos
	// Store this in config.json — ensure the file has restricted permissions on disk
	GitHubToken string `json:"github_token"`

	// DownloadRetries is how many times to retry a failed artifact download
	DownloadRetries int `json:"download_retries"`

	// HealthCheck configures the post-deploy health validation
	HealthCheck HealthCheckConfig `json:"health_check"`

	// Services is the list of Windows services this watcher manages on this server.
	// Each entry describes one Windows service and which binary from the zip it runs.
	// This is the main per-server configuration — different servers can manage
	// different subsets of services (e.g. one server only runs web, another only worker).
	Services []ServiceConfig `json:"services"`

	// NssmPath is the full path to nssm.exe
	// Defaults to "C:\\ProgramData\\chocolatey\\bin\\nssm.exe" if not set
	NssmPath string `json:"nssm_path,omitempty"`

	// LogDir is where watcher writes its own log files
	// Defaults to "<install_dir>\\logs" if not set
	LogDir string `json:"log_dir,omitempty"`
}

// HealthCheckConfig defines how to validate a service after deploy
type HealthCheckConfig struct {
	// Enabled — set to false to skip health checks entirely
	Enabled bool `json:"enabled"`

	// URL is the HTTP endpoint that must return 200 for the deploy to succeed
	// e.g. "http://localhost:8080/health"
	// Can be overridden per-service in ServiceConfig
	URL string `json:"url,omitempty"`

	// Retries is how many times to poll before giving up and rolling back
	Retries int `json:"retries"`

	// IntervalSec is seconds to wait between retries
	IntervalSec int `json:"interval_sec"`

	// TimeoutSec is the HTTP request timeout per attempt
	TimeoutSec int `json:"timeout_sec"`
}

// ServiceConfig describes one Windows service managed by the watcher
type ServiceConfig struct {
	// WindowsServiceName is the name of the Windows service (managed by NSSM)
	// e.g. "admin-be-web-1"
	WindowsServiceName string `json:"windows_service_name"`

	// BinaryName is the filename inside the release zip to use for this service
	// e.g. "web.exe" or "worker.exe"
	BinaryName string `json:"binary_name"`

	// EnvFile is the path to the .env file for this service instance
	// e.g. "C:\\admin-be\\.env.web.1"
	EnvFile string `json:"env_file"`

	// HealthCheckURL overrides the top-level health check URL for this service
	// Leave empty to use the top-level URL (or skip if health check is disabled)
	HealthCheckURL string `json:"health_check_url,omitempty"`

	// AppDir overrides the working directory for this service
	// Defaults to InstallDir/current if not set
	AppDir string `json:"app_dir,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config %q: %w", path, err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Apply defaults
	if cfg.CheckIntervalSec <= 0 {
		cfg.CheckIntervalSec = 60
	}
	if cfg.DownloadRetries <= 0 {
		cfg.DownloadRetries = 3
	}
	if cfg.HealthCheck.Retries <= 0 {
		cfg.HealthCheck.Retries = 10
	}
	if cfg.HealthCheck.IntervalSec <= 0 {
		cfg.HealthCheck.IntervalSec = 3
	}
	if cfg.HealthCheck.TimeoutSec <= 0 {
		cfg.HealthCheck.TimeoutSec = 5
	}
	if cfg.NssmPath == "" {
		cfg.NssmPath = `C:\ProgramData\chocolatey\bin\nssm.exe`
	}
	if cfg.LogDir == "" {
		cfg.LogDir = cfg.InstallDir + `\logs`
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required")
	}
	if c.InstallDir == "" {
		return fmt.Errorf("install_dir is required")
	}
	if c.MetadataURL == "" {
		return fmt.Errorf("metadata_url is required")
	}
	if c.GitHubToken == "" {
		return fmt.Errorf("github_token is required (private repo)")
	}
	if len(c.Services) == 0 {
		return fmt.Errorf("at least one service entry is required")
	}
	for i, svc := range c.Services {
		if svc.WindowsServiceName == "" {
			return fmt.Errorf("services[%d]: windows_service_name is required", i)
		}
		if svc.BinaryName == "" {
			return fmt.Errorf("services[%d]: binary_name is required", i)
		}
		if svc.EnvFile == "" {
			return fmt.Errorf("services[%d]: env_file is required", i)
		}
	}
	return nil
}