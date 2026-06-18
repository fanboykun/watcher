package database

import (
	"time"
)

// AuthCredential stores the single dashboard/API password hash.
type AuthCredential struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	PasswordHash         string `gorm:"not null" json:"-"`
	UsingDefaultPassword bool   `gorm:"not null;default:false" json:"using_default_password"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Watcher represents a repository being monitored for releases.
type Watcher struct {
	ID                    uint   `gorm:"primaryKey" json:"id"`
	Name                  string `gorm:"not null" json:"name"`
	ServiceName           string `gorm:"not null;index" json:"service_name"`
	MetadataURL           string `gorm:"not null" json:"metadata_url"`
	ReleaseRef            string `gorm:"not null;default:'latest'" json:"release_ref"`
	DeploymentEnvironment string `gorm:"not null;default:''" json:"deployment_environment"`
	GitHubToken           string `gorm:"column:github_token;not null;default:''" json:"-"`
	CheckIntervalSec      int    `gorm:"not null;default:300" json:"check_interval_sec"`
	DownloadRetries       int    `gorm:"not null;default:3" json:"download_retries"`
	InstallDir            string `gorm:"not null" json:"install_dir"`
	Paused                bool   `gorm:"not null;default:false" json:"paused"`
	MaxKeptVersions       int    `gorm:"not null;default:3" json:"max_kept_versions"`

	// Health check settings (flattened)
	HcEnabled     bool   `gorm:"not null;default:false" json:"hc_enabled"`
	HcURL         string `gorm:"not null;default:''" json:"hc_url"`
	HcRetries     int    `gorm:"not null;default:10" json:"hc_retries"`
	HcIntervalSec int    `gorm:"not null;default:3" json:"hc_interval_sec"`
	HcTimeoutSec  int    `gorm:"not null;default:5" json:"hc_timeout_sec"`

	// Deploy state (replaces version.txt + state.json)
	CurrentVersion    string     `gorm:"not null;default:''" json:"current_version"`
	MaxIgnoredVersion string     `gorm:"not null;default:''" json:"max_ignored_version"`
	Status            string     `gorm:"not null;default:'unknown'" json:"status"`
	LastChecked       *time.Time `json:"last_checked"`
	LastDeployed      *time.Time `json:"last_deployed"`
	LastError         string     `gorm:"not null;default:''" json:"last_error"`

	WebhookEnabled                  bool       `gorm:"not null;default:false" json:"webhook_enabled"`
	WebhookURL                      string     `gorm:"not null;default:''" json:"webhook_url"`
	WebhookBearerToken              string     `gorm:"not null;default:''" json:"-"`
	WebhookAutoPauseEnabledOverride *bool      `json:"webhook_auto_pause_enabled_override,omitempty"`
	WebhookAutoPauseAfterFailures   *int       `json:"webhook_auto_pause_after_failures_override,omitempty"`
	WebhookPausedAt                 *time.Time `json:"webhook_paused_at,omitempty"`
	WebhookPauseReason              string     `gorm:"not null;default:''" json:"webhook_pause_reason"`
	WebhookFailureStreak            int        `gorm:"not null;default:0" json:"webhook_failure_streak"`
	NotifyVersionFound              bool       `gorm:"not null;default:false" json:"notify_version_found"`
	NotifyDeploymentSucceeded       bool       `gorm:"not null;default:false" json:"notify_deployment_succeeded"`
	NotifyDeploymentFailed          bool       `gorm:"not null;default:false" json:"notify_deployment_failed"`
	NotifyRollbackSucceeded         bool       `gorm:"not null;default:false" json:"notify_rollback_succeeded"`
	NotifyRollbackFailed            bool       `gorm:"not null;default:false" json:"notify_rollback_failed"`
	NotifyServiceHealthChanged      bool       `gorm:"not null;default:false" json:"notify_service_health_changed"`

	// Relations
	Services          []Service         `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"services"`
	DeployLogs        []DeployLog       `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"deploy_logs"`
	PollEvents        []PollEvent       `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"poll_events"`
	WebhookEvents     []WebhookEvent    `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"-"`
	WebhookDeliveries []WebhookDelivery `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"-"`

	// Derived response-only fields
	HasGitHubToken           bool   `gorm:"-" json:"has_github_token"`
	GitHubTokenMasked        string `gorm:"-" json:"github_token_masked,omitempty"`
	HasWebhookBearerToken    bool   `gorm:"-" json:"has_webhook_bearer_token"`
	WebhookBearerTokenMasked string `gorm:"-" json:"webhook_bearer_token_masked,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service represents a managed NSSM or IIS application belonging to a Watcher.
type Service struct {
	ID                   uint                `gorm:"primaryKey" json:"id"`
	WatcherID            uint                `gorm:"not null;index" json:"watcher_id"`
	ServiceType          string              `gorm:"not null;default:'nssm'" json:"service_type"` // "nssm" or "iis"
	WindowsServiceName   string              `gorm:"not null" json:"windows_service_name"`
	BinaryName           string              `gorm:"not null;default:''" json:"binary_name"` // NSSM only
	StartArguments       string              `gorm:"not null;default:''" json:"start_arguments"`
	EnvFile              string              `gorm:"not null;default:''" json:"env_file"` // NSSM only
	HealthCheckURL       string              `gorm:"not null;default:''" json:"health_check_url"`
	IISAppKind           string              `gorm:"not null;default:'static'" json:"iis_app_kind"`  // IIS only: static | php | aspnet_classic
	IISAppPool           string              `gorm:"not null;default:''" json:"iis_app_pool"`        // IIS-hosted only
	IISSiteName          string              `gorm:"not null;default:''" json:"iis_site_name"`       // IIS-hosted only
	IISManagedRuntime    string              `gorm:"not null;default:''" json:"iis_managed_runtime"` // IIS-hosted only; maintained from app kind for compatibility
	PublicURL            string              `gorm:"not null;default:''" json:"public_url"`
	EnvContent           string              `gorm:"type:text" json:"env_content"`
	LastHealthStatus     string              `gorm:"not null;default:''" json:"last_health_status"`
	LastHealthHTTPStatus int                 `gorm:"not null;default:0" json:"last_health_http_status"`
	LastHealthError      string              `gorm:"not null;default:''" json:"last_health_error"`
	LastHealthCheckedAt  *time.Time          `json:"last_health_checked_at,omitempty"`
	ConfigFiles          []ServiceConfigFile `gorm:"foreignKey:ServiceID;constraint:OnDelete:CASCADE" json:"config_files"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServiceConfigFile stores additional managed config files for a service.
type ServiceConfigFile struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ServiceID uint   `gorm:"not null;index" json:"service_id"`
	FilePath  string `gorm:"not null" json:"file_path"`
	Target    string `gorm:"not null;default:'app_dir'" json:"target"` // app_dir or release_dir
	Content   string `gorm:"type:text" json:"content"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeployLog records each deploy attempt for history/timeline.
type DeployLog struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	WatcherID           uint       `gorm:"not null;index" json:"watcher_id"`
	TriggeredBy         string     `gorm:"not null;default:'agent'" json:"triggered_by"` // agent | manual
	Kind                string     `gorm:"not null;default:'deploy'" json:"kind"`        // deploy | rollback
	Reason              string     `gorm:"not null;default:'new_version'" json:"reason"` // new_version | manual_redeploy | manual_rollback | auto_after_deploy_failure
	Version             string     `gorm:"not null" json:"version"`
	FromVersion         string     `gorm:"not null;default:''" json:"from_version"`
	FailedTargetVersion string     `gorm:"not null;default:''" json:"failed_target_version"`
	Status              string     `gorm:"not null" json:"status"`
	Error               string     `gorm:"not null;default:''" json:"error"`
	FailurePhase        string     `gorm:"not null;default:''" json:"failure_phase"`
	DurationMs          int64      `gorm:"not null;default:0" json:"duration_ms"`
	Logs                string     `gorm:"type:text" json:"logs"`
	GitHubDeploymentID  int64      `gorm:"not null;default:0" json:"github_deployment_id"`
	ParentAttemptID     *uint      `gorm:"index" json:"parent_attempt_id,omitempty"`
	RootAttemptID       *uint      `gorm:"index" json:"root_attempt_id,omitempty"`
	StartedAt           *time.Time `json:"started_at"`
	CompletedAt         *time.Time `json:"completed_at"`
}

// HealthEvent records each health check attempt for a service.
type HealthEvent struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ServiceID      uint       `gorm:"not null;index" json:"service_id"`
	Status         string     `gorm:"not null" json:"status"` // healthy|unhealthy|error
	PreviousStatus string     `gorm:"not null;default:''" json:"previous_status"`
	Source         string     `gorm:"not null;default:'manual'" json:"source"` // manual|deploy|rollback|monitor
	HTTPStatus     int        `gorm:"not null;default:0" json:"http_status"`
	Error          string     `gorm:"not null;default:''" json:"error"`
	CheckedAt      *time.Time `json:"checked_at"`
}

// PollEvent records the outcome of a GitHub version check.
type PollEvent struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	WatcherID     uint      `gorm:"not null;index" json:"watcher_id"`
	CheckedAt     time.Time `gorm:"not null;autoCreateTime" json:"checked_at"`
	Status        string    `gorm:"not null" json:"status"` // e.g. "new_release", "no_update", "error"
	RemoteVersion string    `gorm:"not null;default:''" json:"remote_version"`
	Error         string    `json:"error"`
}

type WebhookEvent struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	WatcherID      uint       `gorm:"not null;index" json:"watcher_id"`
	EventID        string     `gorm:"not null;uniqueIndex" json:"event_id"`
	SchemaVersion  string     `gorm:"not null;default:'v1'" json:"schema_version"`
	EventType      string     `gorm:"not null;index" json:"event_type"`
	DedupeKey      string     `gorm:"not null;default:'';index" json:"dedupe_key"`
	Status         string     `gorm:"not null;default:'pending'" json:"status"` // pending|delivered|suppressed|exhausted
	Summary        string     `gorm:"not null;default:''" json:"summary"`
	Payload        string     `gorm:"type:text;not null" json:"payload"`
	OccurredAt     time.Time  `gorm:"not null;index" json:"occurred_at"`
	SuppressedAt   *time.Time `json:"suppressed_at,omitempty"`
	LastDeliveryAt *time.Time `json:"last_delivery_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type WebhookDelivery struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	WatcherID          uint       `gorm:"not null;index" json:"watcher_id"`
	WebhookEventID     uint       `gorm:"not null;index" json:"webhook_event_id"`
	DeliveryID         string     `gorm:"not null;uniqueIndex" json:"delivery_id"`
	Status             string     `gorm:"not null;default:'pending'" json:"status"` // pending|retry_wait|succeeded|failed
	AttemptNumber      int        `gorm:"not null;default:1" json:"attempt_number"`
	ResponseStatusCode int        `gorm:"not null;default:0" json:"response_status_code"`
	ResponseBody       string     `gorm:"type:text" json:"response_body"`
	Error              string     `gorm:"not null;default:''" json:"error"`
	ResolvedURL        string     `gorm:"not null;default:''" json:"resolved_url"`
	AuthType           string     `gorm:"not null;default:'bearer'" json:"auth_type"`
	TokenSource        string     `gorm:"not null;default:''" json:"token_source"`
	NextRetryAt        *time.Time `json:"next_retry_at,omitempty"`
	LastAttemptAt      *time.Time `json:"last_attempt_at,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	ReplayedAt         *time.Time `json:"replayed_at,omitempty"`
	ReplayedBy         string     `gorm:"not null;default:''" json:"replayed_by"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}
