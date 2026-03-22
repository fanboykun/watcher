package api

// ── Watcher DTOs ──────────────────────────────────────────────

// CreateWatcherRequest is the body for POST /api/watchers
type CreateWatcherRequest struct {
	Name             string                 `json:"name" binding:"required"`
	ServiceName      string                 `json:"service_name" binding:"required"`
	MetadataURL      string                 `json:"metadata_url" binding:"required"`
	CheckIntervalSec int                    `json:"check_interval_sec"`
	DownloadRetries  int                    `json:"download_retries"`
	InstallDir       string                 `json:"install_dir" binding:"required"`
	HcEnabled        bool                   `json:"hc_enabled"`
	HcURL            string                 `json:"hc_url"`
	HcRetries        int                    `json:"hc_retries"`
	HcIntervalSec    int                    `json:"hc_interval_sec"`
	HcTimeoutSec     int                    `json:"hc_timeout_sec"`
	Services         []CreateServiceRequest `json:"services"`
}

// UpdateWatcherRequest is the body for PUT /api/watchers/:id
type UpdateWatcherRequest struct {
	Name             *string `json:"name"`
	ServiceName      *string `json:"service_name"`
	MetadataURL      *string `json:"metadata_url"`
	CheckIntervalSec *int    `json:"check_interval_sec"`
	DownloadRetries  *int    `json:"download_retries"`
	InstallDir       *string `json:"install_dir"`
	HcEnabled        *bool   `json:"hc_enabled"`
	HcURL            *string `json:"hc_url"`
	HcRetries        *int    `json:"hc_retries"`
	HcIntervalSec    *int    `json:"hc_interval_sec"`
	HcTimeoutSec     *int    `json:"hc_timeout_sec"`
}

// ── Service DTOs ──────────────────────────────────────────────

// CreateServiceRequest is the body for POST /api/watchers/:id/services
type CreateServiceRequest struct {
	ServiceType        string `json:"service_type"`                              // "nssm" (default) or "static"
	WindowsServiceName string `json:"windows_service_name" binding:"required"`
	BinaryName         string `json:"binary_name"`                               // NSSM only
	EnvFile            string `json:"env_file"`                                  // NSSM only
	HealthCheckURL     string `json:"health_check_url"`
	IISAppPool         string `json:"iis_app_pool"`                              // Static only
	IISSiteName        string `json:"iis_site_name"`                             // Static only
	PublicURL          string `json:"public_url"`
}

// UpdateServiceRequest is the body for PUT /api/watchers/:id/services/:sid
type UpdateServiceRequest struct {
	ServiceType        *string `json:"service_type"`
	WindowsServiceName *string `json:"windows_service_name"`
	BinaryName         *string `json:"binary_name"`
	EnvFile            *string `json:"env_file"`
	HealthCheckURL     *string `json:"health_check_url"`
	IISAppPool         *string `json:"iis_app_pool"`
	IISSiteName        *string `json:"iis_site_name"`
	PublicURL          *string `json:"public_url"`
}

// ── Response helpers ──────────────────────────────────────────

type ErrorResponse struct {
	Error string `json:"error"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
