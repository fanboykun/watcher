package api

// ── Watcher DTOs ──────────────────────────────────────────────

// CreateWatcherRequest is the body for POST /api/watchers
type CreateWatcherRequest struct {
	Name                            string                 `json:"name" binding:"required"`
	ServiceName                     string                 `json:"service_name" binding:"required"`
	MetadataURL                     string                 `json:"metadata_url" binding:"required"`
	ReleaseRef                      string                 `json:"release_ref"`
	DeploymentEnvironment           string                 `json:"deployment_environment"`
	GitHubToken                     string                 `json:"github_token"`
	CheckIntervalSec                int                    `json:"check_interval_sec"`
	DownloadRetries                 int                    `json:"download_retries"`
	InstallDir                      string                 `json:"install_dir" binding:"required"`
	HcEnabled                       bool                   `json:"hc_enabled"`
	HcURL                           string                 `json:"hc_url"`
	HcRetries                       int                    `json:"hc_retries"`
	HcIntervalSec                   int                    `json:"hc_interval_sec"`
	HcTimeoutSec                    int                    `json:"hc_timeout_sec"`
	Paused                          bool                   `json:"paused"`
	MaxKeptVersions                 int                    `json:"max_kept_versions"`
	WebhookEnabled                  bool                   `json:"webhook_enabled"`
	WebhookURL                      string                 `json:"webhook_url"`
	WebhookBearerToken              string                 `json:"webhook_bearer_token"`
	WebhookAutoPauseEnabledOverride *bool                  `json:"webhook_auto_pause_enabled_override"`
	WebhookAutoPauseAfterFailures   *int                   `json:"webhook_auto_pause_after_failures_override"`
	NotifyVersionFound              bool                   `json:"notify_version_found"`
	NotifyDeploymentSucceeded       bool                   `json:"notify_deployment_succeeded"`
	NotifyDeploymentFailed          bool                   `json:"notify_deployment_failed"`
	NotifyRollbackSucceeded         bool                   `json:"notify_rollback_succeeded"`
	NotifyRollbackFailed            bool                   `json:"notify_rollback_failed"`
	NotifyServiceHealthChanged      bool                   `json:"notify_service_health_changed"`
	Services                        []CreateServiceRequest `json:"services"`
}

// UpdateWatcherRequest is the body for PUT /api/watchers/:id
type UpdateWatcherRequest struct {
	Name                            *string `json:"name"`
	ServiceName                     *string `json:"service_name"`
	MetadataURL                     *string `json:"metadata_url"`
	ReleaseRef                      *string `json:"release_ref"`
	DeploymentEnvironment           *string `json:"deployment_environment"`
	GitHubToken                     *string `json:"github_token"`
	CheckIntervalSec                *int    `json:"check_interval_sec"`
	DownloadRetries                 *int    `json:"download_retries"`
	InstallDir                      *string `json:"install_dir"`
	HcEnabled                       *bool   `json:"hc_enabled"`
	HcURL                           *string `json:"hc_url"`
	HcRetries                       *int    `json:"hc_retries"`
	HcIntervalSec                   *int    `json:"hc_interval_sec"`
	HcTimeoutSec                    *int    `json:"hc_timeout_sec"`
	Paused                          *bool   `json:"paused"`
	MaxKeptVersions                 *int    `json:"max_kept_versions"`
	WebhookEnabled                  *bool   `json:"webhook_enabled"`
	WebhookURL                      *string `json:"webhook_url"`
	WebhookBearerToken              *string `json:"webhook_bearer_token"`
	WebhookAutoPauseEnabledOverride *bool   `json:"webhook_auto_pause_enabled_override"`
	WebhookAutoPauseAfterFailures   *int    `json:"webhook_auto_pause_after_failures_override"`
	NotifyVersionFound              *bool   `json:"notify_version_found"`
	NotifyDeploymentSucceeded       *bool   `json:"notify_deployment_succeeded"`
	NotifyDeploymentFailed          *bool   `json:"notify_deployment_failed"`
	NotifyRollbackSucceeded         *bool   `json:"notify_rollback_succeeded"`
	NotifyRollbackFailed            *bool   `json:"notify_rollback_failed"`
	NotifyServiceHealthChanged      *bool   `json:"notify_service_health_changed"`
}

// ── Service DTOs ──────────────────────────────────────────────

type ConfigFileRequest struct {
	FilePath string `json:"file_path"`
	Target   string `json:"target"`
	Content  string `json:"content"`
}

// CreateServiceRequest is the body for POST /api/watchers/:id/services
type CreateServiceRequest struct {
	ServiceType        string              `json:"service_type"` // "nssm" (default) or "iis"
	WindowsServiceName string              `json:"windows_service_name" binding:"required"`
	BinaryName         string              `json:"binary_name"` // NSSM only
	StartArguments     string              `json:"start_arguments"`
	EnvFile            string              `json:"env_file"` // NSSM only
	HealthCheckURL     string              `json:"health_check_url"`
	IISAppKind         string              `json:"iis_app_kind"`        // IIS only: static | php | aspnet_classic
	IISAppPool         string              `json:"iis_app_pool"`        // IIS-hosted only
	IISSiteName        string              `json:"iis_site_name"`       // IIS-hosted only
	IISManagedRuntime  string              `json:"iis_managed_runtime"` // IIS-hosted only; "" = No Managed Code
	PublicURL          string              `json:"public_url"`
	EnvContent         string              `json:"env_content"`
	ConfigFiles        []ConfigFileRequest `json:"config_files"`
}

// UpdateServiceRequest is the body for PUT /api/watchers/:id/services/:sid
type UpdateServiceRequest struct {
	ServiceType        *string              `json:"service_type"`
	WindowsServiceName *string              `json:"windows_service_name"`
	BinaryName         *string              `json:"binary_name"`
	StartArguments     *string              `json:"start_arguments"`
	EnvFile            *string              `json:"env_file"`
	HealthCheckURL     *string              `json:"health_check_url"`
	IISAppKind         *string              `json:"iis_app_kind"`
	IISAppPool         *string              `json:"iis_app_pool"`
	IISSiteName        *string              `json:"iis_site_name"`
	IISManagedRuntime  *string              `json:"iis_managed_runtime"`
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
	Environment                     string `json:"environment"`
	GitHubDeployEnabled             bool   `json:"github_deploy_enabled"`
	LogDir                          string `json:"log_dir"`
	NssmPath                        string `json:"nssm_path"`
	DBPath                          string `json:"db_path"`
	APIPort                         string `json:"api_port"`
	APIBaseURL                      string `json:"api_base_url"`
	WatcherRepoURL                  string `json:"watcher_repo_url"`
	WatcherServiceName              string `json:"watcher_service_name"`
	HasGitHubToken                  bool   `json:"has_github_token"`
	GitHubTokenMasked               string `json:"github_token_masked"`
	WebhookDefaultURL               string `json:"webhook_default_url"`
	HasWebhookDefaultBearerToken    bool   `json:"has_webhook_default_bearer_token"`
	WebhookDefaultBearerTokenMasked string `json:"webhook_default_bearer_token_masked"`
	WebhookTimeoutSec               int    `json:"webhook_timeout_sec"`
	WebhookRetryScheduleSec         string `json:"webhook_retry_schedule_sec"`
	WebhookAutoPauseEnabled         bool   `json:"webhook_auto_pause_enabled"`
	WebhookAutoPauseAfterFailures   int    `json:"webhook_auto_pause_after_failures"`
	WebhookEventRetentionDays       int    `json:"webhook_event_retention_days"`
	WebhookDeliveryRetentionDays    int    `json:"webhook_delivery_retention_days"`
	EnvPath                         string `json:"env_path"`
}

type UpdateSelfConfigRequest struct {
	Environment                   *string `json:"environment"`
	GitHubToken                   *string `json:"github_token"`
	GitHubDeployEnabled           *bool   `json:"github_deploy_enabled"`
	LogDir                        *string `json:"log_dir"`
	NssmPath                      *string `json:"nssm_path"`
	DBPath                        *string `json:"db_path"`
	APIPort                       *string `json:"api_port"`
	APIBaseURL                    *string `json:"api_base_url"`
	WatcherRepoURL                *string `json:"watcher_repo_url"`
	WatcherServiceName            *string `json:"watcher_service_name"`
	WebhookDefaultURL             *string `json:"webhook_default_url"`
	WebhookDefaultBearerToken     *string `json:"webhook_default_bearer_token"`
	WebhookTimeoutSec             *int    `json:"webhook_timeout_sec"`
	WebhookRetryScheduleSec       *string `json:"webhook_retry_schedule_sec"`
	WebhookAutoPauseEnabled       *bool   `json:"webhook_auto_pause_enabled"`
	WebhookAutoPauseAfterFailures *int    `json:"webhook_auto_pause_after_failures"`
	WebhookEventRetentionDays     *int    `json:"webhook_event_retention_days"`
	WebhookDeliveryRetentionDays  *int    `json:"webhook_delivery_retention_days"`
}

type ResumeWebhookRequest struct {
	ReplaySuppressed bool `json:"replay_suppressed"`
}
