package database

import (
	"time"
)

// Watcher represents a repository being monitored for releases.
type Watcher struct {
	ID                    uint   `gorm:"primaryKey" json:"id"`
	Name                  string `gorm:"not null" json:"name"`
	ServiceName           string `gorm:"not null;uniqueIndex" json:"service_name"`
	MetadataURL           string `gorm:"not null" json:"metadata_url"`
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

	// Relations
	Services   []Service   `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"services"`
	DeployLogs []DeployLog `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"deploy_logs"`
	PollEvents []PollEvent `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"poll_events"`

	// Derived response-only fields
	HasGitHubToken    bool   `gorm:"-" json:"has_github_token"`
	GitHubTokenMasked string `gorm:"-" json:"github_token_masked,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service represents a managed NSSM or IIS application belonging to a Watcher.
type Service struct {
	ID                 uint                `gorm:"primaryKey" json:"id"`
	WatcherID          uint                `gorm:"not null;index" json:"watcher_id"`
	ServiceType        string              `gorm:"not null;default:'nssm'" json:"service_type"` // "nssm" or "static"
	WindowsServiceName string              `gorm:"not null" json:"windows_service_name"`
	BinaryName         string              `gorm:"not null;default:''" json:"binary_name"` // NSSM only
	StartArguments     string              `gorm:"not null;default:''" json:"start_arguments"`
	EnvFile            string              `gorm:"not null;default:''" json:"env_file"` // NSSM only
	HealthCheckURL     string              `gorm:"not null;default:''" json:"health_check_url"`
	IISAppPool         string              `gorm:"not null;default:''" json:"iis_app_pool"`  // Static only
	IISSiteName        string              `gorm:"not null;default:''" json:"iis_site_name"` // Static only
	PublicURL          string              `gorm:"not null;default:''" json:"public_url"`
	EnvContent         string              `gorm:"type:text" json:"env_content"`
	ConfigFiles        []ServiceConfigFile `gorm:"foreignKey:ServiceID;constraint:OnDelete:CASCADE" json:"config_files"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServiceConfigFile stores additional managed config files for a service.
type ServiceConfigFile struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ServiceID uint   `gorm:"not null;index" json:"service_id"`
	FilePath  string `gorm:"not null" json:"file_path"`
	Content   string `gorm:"type:text" json:"content"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DeployLog records each deploy attempt for history/timeline.
type DeployLog struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	WatcherID          uint       `gorm:"not null;index" json:"watcher_id"`
	TriggeredBy        string     `gorm:"not null;default:'agent'" json:"triggered_by"` // agent | manual
	Version            string     `gorm:"not null" json:"version"`
	FromVersion        string     `gorm:"not null;default:''" json:"from_version"`
	Status             string     `gorm:"not null" json:"status"`
	Error              string     `gorm:"not null;default:''" json:"error"`
	DurationMs         int64      `gorm:"not null;default:0" json:"duration_ms"`
	Logs               string     `gorm:"type:text" json:"logs"`
	GitHubDeploymentID int64      `gorm:"not null;default:0" json:"github_deployment_id"`
	StartedAt          *time.Time `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at"`
}

// HealthEvent records each health check attempt for a service.
type HealthEvent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	ServiceID  uint       `gorm:"not null;index" json:"service_id"`
	Status     string     `gorm:"not null" json:"status"` // healthy|unhealthy|error
	HTTPStatus int        `gorm:"not null;default:0" json:"http_status"`
	Error      string     `gorm:"not null;default:''" json:"error"`
	CheckedAt  *time.Time `json:"checked_at"`
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
