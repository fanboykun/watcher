package internal

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config is the top-level watcher config loaded from config.json.
// A single watcher instance can watch multiple repos, each with their
// own metadata URL, service name, poll interval, and services list.
type Config struct {
	// Environment is a human-readable label for this server (informational only)
	Environment string `json:"environment"`

	// GitHubToken is a PAT with repo scope, shared across all watched repos
	// Must belong to an account with read access to all repos being watched
	GitHubToken string `json:"github_token"`

	// LogDir is where watcher writes its own logs
	// Defaults to "C:\\apps\\watcher\\logs" if not set
	LogDir string `json:"log_dir,omitempty"`

	// NssmPath is the full path to nssm.exe
	// Defaults to "C:\\ProgramData\\chocolatey\\bin\\nssm.exe" if not set
	NssmPath string `json:"nssm_path,omitempty"`

	// Watchers is the list of repos this agent watches.
	// Each entry is fully independent — its own repo, poll interval, and services.
	Watchers []WatcherConfig `json:"watchers"`
}

// WatcherConfig defines one watched repo and the services it manages
type WatcherConfig struct {
	// Name is a human-readable label for this watcher entry (used in logs)
	// e.g. "admin-be", "payments-service"
	Name string `json:"name"`

	// ServiceName must match the key inside version.json published by this repo's release workflow
	// e.g. "admin-be"
	ServiceName string `json:"service_name"`

	// MetadataURL is the URL to version.json for this repo
	// e.g. "https://github.com/your-org/admin-be/releases/latest/download/version.json"
	MetadataURL string `json:"metadata_url"`

	// CheckIntervalSec is how often to poll this specific repo (in seconds)
	CheckIntervalSec int `json:"check_interval_sec"`

	// DownloadRetries is how many times to retry a failed artifact download
	DownloadRetries int `json:"download_retries,omitempty"`

	// InstallDir is the base directory for releases of this service on this machine
	// e.g. "C:\\apps\\admin-be"
	InstallDir string `json:"install_dir"`

	// HealthCheck configures post-deploy health validation for this watcher
	HealthCheck HealthCheckConfig `json:"health_check"`

	// Services is the list of Windows services this watcher entry manages.
	// All services in this list are deployed together when a new version is detected.
	Services []ServiceConfig `json:"services"`
}

// HealthCheckConfig defines how to validate services after deploy
type HealthCheckConfig struct {
	// Enabled — set false to skip health checks entirely for this watcher
	Enabled bool `json:"enabled"`

	// URL is the default health endpoint — can be overridden per service
	URL string `json:"url,omitempty"`

	// Retries is how many times to poll before rolling back
	Retries int `json:"retries"`

	// IntervalSec is seconds between retries
	IntervalSec int `json:"interval_sec"`

	// TimeoutSec is the per-request HTTP timeout
	TimeoutSec int `json:"timeout_sec"`
}

// ServiceConfig describes one Windows service managed by a watcher entry
type ServiceConfig struct {
	// WindowsServiceName is the NSSM-managed Windows service name
	// e.g. "admin-be-web-1"
	WindowsServiceName string `json:"windows_service_name"`

	// BinaryName is the filename inside the release zip
	// e.g. "web.exe" or "worker.exe"
	BinaryName string `json:"binary_name"`

	// EnvFile is the .env file path for this service instance
	// e.g. "C:\\admin-be\\.env.web.1"
	EnvFile string `json:"env_file"`

	// HealthCheckURL overrides the watcher-level health check URL for this service
	HealthCheckURL string `json:"health_check_url,omitempty"`
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

	// Apply defaults across all watcher entries
	for i := range cfg.Watchers {
		w := &cfg.Watchers[i]
		if w.Name == "" {
			w.Name = w.ServiceName
		}
		if w.CheckIntervalSec <= 0 {
			w.CheckIntervalSec = 60
		}
		if w.DownloadRetries <= 0 {
			w.DownloadRetries = 3
		}
		if w.HealthCheck.Retries <= 0 {
			w.HealthCheck.Retries = 10
		}
		if w.HealthCheck.IntervalSec <= 0 {
			w.HealthCheck.IntervalSec = 3
		}
		if w.HealthCheck.TimeoutSec <= 0 {
			w.HealthCheck.TimeoutSec = 5
		}
	}
	if cfg.NssmPath == "" {
		cfg.NssmPath = `C:\ProgramData\chocolatey\bin\nssm.exe`
	}
	if cfg.LogDir == "" {
		cfg.LogDir = `C:\apps\watcher\logs`
	}

	return &cfg, cfg.validate()
}

func (c *Config) validate() error {
	if c.GitHubToken == "" {
		return fmt.Errorf("github_token is required")
	}
	if len(c.Watchers) == 0 {
		return fmt.Errorf("at least one watcher entry is required")
	}
	for i, w := range c.Watchers {
		if w.ServiceName == "" {
			return fmt.Errorf("watchers[%d]: service_name is required", i)
		}
		if w.MetadataURL == "" {
			return fmt.Errorf("watchers[%d] %q: metadata_url is required", i, w.Name)
		}
		if w.InstallDir == "" {
			return fmt.Errorf("watchers[%d] %q: install_dir is required", i, w.Name)
		}
		if len(w.Services) == 0 {
			return fmt.Errorf("watchers[%d] %q: at least one service is required", i, w.Name)
		}
		for j, svc := range w.Services {
			if svc.WindowsServiceName == "" {
				return fmt.Errorf("watchers[%d].services[%d]: windows_service_name is required", i, j)
			}
			if svc.BinaryName == "" {
				return fmt.Errorf("watchers[%d].services[%d]: binary_name is required", i, j)
			}
			if svc.EnvFile == "" {
				return fmt.Errorf("watchers[%d].services[%d]: env_file is required", i, j)
			}
		}
	}
	return nil
}