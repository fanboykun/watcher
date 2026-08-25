package api

import (
	"fmt"
	"strings"

	"github.com/fanboykun/watcher/internal/database"
)

func normalizeServiceType(t string) string {
	switch strings.TrimSpace(strings.ToLower(t)) {
	case "", "nssm":
		return "nssm"
	case "iis", "static":
		return "iis"
	default:
		return strings.TrimSpace(t)
	}
}

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

func normalizeIISManagedRuntime(runtime string) string {
	switch strings.TrimSpace(strings.ToLower(runtime)) {
	case "", "none", "no-managed-code", "no managed code":
		return ""
	case "v2", "v2.0":
		return "v2.0"
	case "v4", "v4.0", ".net clr v4.0":
		return "v4.0"
	default:
		return strings.TrimSpace(runtime)
	}
}

func resolvedIISManagedRuntime(appKind, runtime string) string {
	switch normalizeIISAppKind(appKind, runtime) {
	case "php", "static":
		return ""
	case "aspnet_classic":
		if normalized := normalizeIISManagedRuntime(runtime); normalized != "" {
			return normalized
		}
		return "v4.0"
	default:
		return normalizeIISManagedRuntime(runtime)
	}
}

func validateServicePayload(svc *database.Service) error {
	if svc == nil {
		return fmt.Errorf("service payload is required")
	}

	svc.ServiceType = normalizeServiceType(svc.ServiceType)
	svc.WindowsServiceName = strings.TrimSpace(svc.WindowsServiceName)
	svc.BinaryName = strings.TrimSpace(svc.BinaryName)
	svc.EnvFile = strings.TrimSpace(svc.EnvFile)
	svc.HealthCheckURL = strings.TrimSpace(svc.HealthCheckURL)
	svc.IISAppKind = normalizeIISAppKind(svc.IISAppKind, svc.IISManagedRuntime)
	svc.IISAppPool = strings.TrimSpace(svc.IISAppPool)
	svc.IISSiteName = strings.TrimSpace(svc.IISSiteName)
	svc.PublicURL = strings.TrimSpace(svc.PublicURL)
	svc.IISManagedRuntime = resolvedIISManagedRuntime(svc.IISAppKind, svc.IISManagedRuntime)

	if svc.WindowsServiceName == "" {
		return fmt.Errorf("windows_service_name is required")
	}

	switch svc.ServiceType {
	case "nssm":
		if svc.BinaryName == "" {
			return fmt.Errorf("binary_name is required for NSSM services")
		}
	case "iis":
		if svc.IISAppKind != "static" && svc.IISAppKind != "php" && svc.IISAppKind != "aspnet_classic" {
			return fmt.Errorf("iis_app_kind must be one of: static, php, aspnet_classic")
		}
		if svc.IISAppPool == "" {
			svc.IISAppPool = svc.WindowsServiceName
		}
		if svc.IISSiteName == "" {
			svc.IISSiteName = svc.WindowsServiceName
		}
		svc.BinaryName = ""
		svc.EnvFile = ""
	default:
		return fmt.Errorf("service_type must be one of: nssm, iis")
	}

	return nil
}

func defaultReleaseRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "latest"
	}
	return ref
}

func normalizeConfigFileTarget(target string) string {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "", "app", "app_dir", "install_dir":
		return "app_dir"
	case "release", "release_dir", "current":
		return "release_dir"
	default:
		return "app_dir"
	}
}

// Handler holds dependencies for all API endpoints.
