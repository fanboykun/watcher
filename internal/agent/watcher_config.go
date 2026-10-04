package agent

import (
	"strings"

	"github.com/fanboykun/watcher/internal/database"
)

// WatcherConfig is the in-memory representation used by the deploy pipeline.
type WatcherConfig struct {
	Name                  string
	ServiceName           string
	MetadataURL           string
	ReleaseRef            string
	DeploymentEnvironment string
	GitHubToken           string
	CheckIntervalSec      int
	DownloadRetries       int
	InstallDir            string
	Paused                bool
	AutoDeploy            bool
	InterceptNextRelease  bool
	PendingVersion        string
	MaxKeptVersions       int
	HealthCheck           HealthCheckConfig
	Services              []ServiceConfig
}

type HealthCheckConfig struct {
	Enabled     bool
	URL         string
	Retries     int
	IntervalSec int
	TimeoutSec  int
}

type ConfigFile struct {
	FilePath string
	Target   string
	Content  string
}

type ServiceConfig struct {
	ID                 uint
	ServiceType        string
	WindowsServiceName string
	BinaryName         string
	StartArguments     string
	EnvFile            string
	EnvContent         string
	HealthCheckURL     string
	IISAppKind         string
	IISAppPool         string
	IISSiteName        string
	IISManagedRuntime  string
	PublicURL          string
	ConfigFiles        []ConfigFile
}

// WatcherConfigFromDB converts persisted watcher settings for the deploy pipeline.
func WatcherConfigFromDB(w *database.Watcher) *WatcherConfig {
	releaseRef := strings.TrimSpace(w.ReleaseRef)
	if releaseRef == "" {
		releaseRef = "latest"
	}
	cfg := &WatcherConfig{
		Name:                  w.Name,
		ServiceName:           w.ServiceName,
		MetadataURL:           w.MetadataURL,
		ReleaseRef:            releaseRef,
		DeploymentEnvironment: strings.TrimSpace(w.DeploymentEnvironment),
		GitHubToken:           strings.TrimSpace(w.GitHubToken),
		CheckIntervalSec:      w.CheckIntervalSec,
		DownloadRetries:       w.DownloadRetries,
		InstallDir:            w.InstallDir,
		Paused:                w.Paused,
		AutoDeploy:            w.AutoDeploy,
		InterceptNextRelease:  w.InterceptNextRelease,
		PendingVersion:        w.PendingVersion,
		MaxKeptVersions:       max(w.MaxKeptVersions, 1),
		HealthCheck: HealthCheckConfig{
			Enabled:     w.HcEnabled,
			URL:         w.HcURL,
			Retries:     w.HcRetries,
			IntervalSec: w.HcIntervalSec,
			TimeoutSec:  w.HcTimeoutSec,
		},
	}
	for _, s := range w.Services {
		svc := ServiceConfig{
			ID:                 s.ID,
			ServiceType:        normalizeServiceType(s.ServiceType),
			WindowsServiceName: s.WindowsServiceName,
			BinaryName:         s.BinaryName,
			StartArguments:     s.StartArguments,
			EnvFile:            s.EnvFile,
			EnvContent:         s.EnvContent,
			HealthCheckURL:     s.HealthCheckURL,
			IISAppKind:         normalizeIISAppKind(s.IISAppKind, s.IISManagedRuntime),
			IISAppPool:         s.IISAppPool,
			IISSiteName:        s.IISSiteName,
			IISManagedRuntime:  s.IISManagedRuntime,
			PublicURL:          s.PublicURL,
		}
		for _, file := range s.ConfigFiles {
			svc.ConfigFiles = append(svc.ConfigFiles, ConfigFile{
				FilePath: file.FilePath,
				Target:   file.Target,
				Content:  file.Content,
			})
		}
		cfg.Services = append(cfg.Services, svc)
	}
	return cfg
}

// normalizeServiceType normalizes service type.
func normalizeServiceType(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case "", "nssm":
		return "nssm"
	case "iis", "static":
		return "iis"
	default:
		return strings.TrimSpace(raw)
	}
}

// normalizeIISAppKind normalizes iis app kind.
func normalizeIISAppKind(kind, runtime string) string {
	switch strings.TrimSpace(strings.ToLower(kind)) {
	case "", "static":
		if normalizeIISManagedRuntime(runtime) != "" {
			return "aspnet_classic"
		}
		return "static"
	case "php":
		return "php"
	case "aspnet_classic", "aspnet-classic", "aspnet", "asp.net", "asp.net_classic":
		return "aspnet_classic"
	default:
		return strings.TrimSpace(kind)
	}
}
