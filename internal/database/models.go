package database

import (
	"time"

	"gorm.io/gorm"
)

// Watcher represents a watched repository and its deploy state.
type Watcher struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	Name             string         `gorm:"not null" json:"name"`
	ServiceName      string         `gorm:"not null;uniqueIndex" json:"service_name"`
	MetadataURL      string         `gorm:"not null" json:"metadata_url"`
	CheckIntervalSec int            `gorm:"not null;default:60" json:"check_interval_sec"`
	DownloadRetries  int            `gorm:"not null;default:3" json:"download_retries"`
	InstallDir       string         `gorm:"not null" json:"install_dir"`

	// Health check settings (flattened)
	HcEnabled     bool   `gorm:"not null;default:false" json:"hc_enabled"`
	HcURL         string `gorm:"not null;default:''" json:"hc_url"`
	HcRetries     int    `gorm:"not null;default:10" json:"hc_retries"`
	HcIntervalSec int    `gorm:"not null;default:3" json:"hc_interval_sec"`
	HcTimeoutSec  int    `gorm:"not null;default:5" json:"hc_timeout_sec"`

	// Deploy state (replaces version.txt + state.json)
	CurrentVersion string     `gorm:"not null;default:''" json:"current_version"`
	Status         string     `gorm:"not null;default:'unknown'" json:"status"`
	LastChecked    *time.Time `json:"last_checked"`
	LastDeployed   *time.Time `json:"last_deployed"`
	LastError      string     `gorm:"not null;default:''" json:"last_error"`

	// Relations
	Services   []Service   `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"services"`
	DeployLogs []DeployLog `gorm:"foreignKey:WatcherID;constraint:OnDelete:CASCADE" json:"deploy_logs,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Service represents a deployable unit under a watcher.
// ServiceType determines how the service is managed:
//   - "nssm"   = binary process managed by NSSM (default)
//   - "static" = static files served by IIS
type Service struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	WatcherID          uint   `gorm:"not null;index" json:"watcher_id"`
	ServiceType        string `gorm:"not null;default:'nssm'" json:"service_type"` // "nssm" or "static"
	WindowsServiceName string `gorm:"not null" json:"windows_service_name"`
	BinaryName         string `gorm:"not null;default:''" json:"binary_name"`  // NSSM only
	EnvFile            string `gorm:"not null;default:''" json:"env_file"`     // NSSM only
	HealthCheckURL     string `gorm:"not null;default:''" json:"health_check_url"`
	IISAppPool         string `gorm:"not null;default:''" json:"iis_app_pool"`  // Static only
	IISSiteName        string `gorm:"not null;default:''" json:"iis_site_name"` // Static only

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// DeployLog records each deploy attempt for history/timeline.
type DeployLog struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	WatcherID   uint       `gorm:"not null;index" json:"watcher_id"`
	Version     string     `gorm:"not null" json:"version"`
	FromVersion string     `gorm:"not null;default:''" json:"from_version"`
	Status      string     `gorm:"not null" json:"status"`
	Error       string     `gorm:"not null;default:''" json:"error"`
	DurationMs  int64      `gorm:"not null;default:0" json:"duration_ms"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
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
