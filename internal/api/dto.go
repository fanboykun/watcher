package api

// ── Watcher DTOs ──────────────────────────────────────────────

// CreateWatcherRequest is the body for POST /api/watchers
type CreateWatcherRequest struct {
	Name                  string                 `json:"name" binding:"required"`
	ServiceName           string                 `json:"service_name" binding:"required"`
	MetadataURL           string                 `json:"metadata_url" binding:"required"`
	DeploymentEnvironment string                 `json:"deployment_environment"`
	GitHubToken           string                 `json:"github_token"`
	CheckIntervalSec      int                    `json:"check_interval_sec"`
	DownloadRetries       int                    `json:"download_retries"`
	InstallDir            string                 `json:"install_dir" binding:"required"`
	HcEnabled             bool                   `json:"hc_enabled"`
	HcURL                 string                 `json:"hc_url"`
	HcRetries             int                    `json:"hc_retries"`
	HcIntervalSec         int                    `json:"hc_interval_sec"`
	HcTimeoutSec          int                    `json:"hc_timeout_sec"`
	Paused                bool                   `json:"paused"`
	MaxKeptVersions       int                    `json:"max_kept_versions"`
	Services              []CreateServiceRequest `json:"services"`
}

// UpdateWatcherRequest is the body for PUT /api/watchers/:id
type UpdateWatcherRequest struct {
	Name                  *string `json:"name"`
	ServiceName           *string `json:"service_name"`
	MetadataURL           *string `json:"metadata_url"`
	DeploymentEnvironment *string `json:"deployment_environment"`
	GitHubToken           *string `json:"github_token"`
	CheckIntervalSec      *int    `json:"check_interval_sec"`
	DownloadRetries       *int    `json:"download_retries"`
	InstallDir            *string `json:"install_dir"`
	HcEnabled             *bool   `json:"hc_enabled"`
	HcURL                 *string `json:"hc_url"`
	HcRetries             *int    `json:"hc_retries"`
	HcIntervalSec         *int    `json:"hc_interval_sec"`
	HcTimeoutSec          *int    `json:"hc_timeout_sec"`
	Paused                *bool   `json:"paused"`
	MaxKeptVersions       *int    `json:"max_kept_versions"`
}

// ── Service DTOs ──────────────────────────────────────────────

type ConfigFileRequest struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

// CreateServiceRequest is the body for POST /api/watchers/:id/services
type CreateServiceRequest struct {
	ServiceType        string              `json:"service_type"` // "nssm" (default) or "static"
	WindowsServiceName string              `json:"windows_service_name" binding:"required"`
	BinaryName         string              `json:"binary_name"` // NSSM only
	EnvFile            string              `json:"env_file"`    // NSSM only
	HealthCheckURL     string              `json:"health_check_url"`
	IISAppPool         string              `json:"iis_app_pool"`  // Static only
	IISSiteName        string              `json:"iis_site_name"` // Static only
	PublicURL          string              `json:"public_url"`
	EnvContent         string              `json:"env_content"`
	ConfigFiles        []ConfigFileRequest `json:"config_files"`
}

// UpdateServiceRequest is the body for PUT /api/watchers/:id/services/:sid
type UpdateServiceRequest struct {
	ServiceType        *string              `json:"service_type"`
	WindowsServiceName *string              `json:"windows_service_name"`
	BinaryName         *string              `json:"binary_name"`
	EnvFile            *string              `json:"env_file"`
	HealthCheckURL     *string              `json:"health_check_url"`
	IISAppPool         *string              `json:"iis_app_pool"`
	IISSiteName        *string              `json:"iis_site_name"`
	PublicURL          *string              `json:"public_url"`
	EnvContent         *string              `json:"env_content"`
	ConfigFiles        *[]ConfigFileRequest `json:"config_files"`
}

// ── Response helpers ──────────────────────────────────────────

type ErrorResponse struct {
	Error string `json:"error"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// ── Rollback DTOs ─────────────────────────────────────────────

type RollbackRequest struct {
	Version      string `json:"version" binding:"required"`
	ReportGitHub *bool  `json:"report_github"`
}

// ── Agent self config DTOs ───────────────────────────────────

type SelfConfigResponse struct {
	Environment         string `json:"environment"`
	GitHubDeployEnabled bool   `json:"github_deploy_enabled"`
	LogDir              string `json:"log_dir"`
	NssmPath            string `json:"nssm_path"`
	DBPath              string `json:"db_path"`
	APIPort             string `json:"api_port"`
	APIBaseURL          string `json:"api_base_url"`
	WatcherRepoURL      string `json:"watcher_repo_url"`
	WatcherServiceName  string `json:"watcher_service_name"`
	HasGitHubToken      bool   `json:"has_github_token"`
	GitHubTokenMasked   string `json:"github_token_masked"`
	EnvPath             string `json:"env_path"`
}

type UpdateSelfConfigRequest struct {
	Environment         *string `json:"environment"`
	GitHubToken         *string `json:"github_token"`
	GitHubDeployEnabled *bool   `json:"github_deploy_enabled"`
	LogDir              *string `json:"log_dir"`
	NssmPath            *string `json:"nssm_path"`
	DBPath              *string `json:"db_path"`
	APIPort             *string `json:"api_port"`
	APIBaseURL          *string `json:"api_base_url"`
	WatcherRepoURL      *string `json:"watcher_repo_url"`
	WatcherServiceName  *string `json:"watcher_service_name"`
}
