package api

import (
	"fmt"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
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

// resolveWatcherSource validates source settings together, because what
// metadata_url and release_ref may hold depends on the source type.
func resolveWatcherSource(sourceType, metadataURL, releaseRef string) (normalizedType, normalizedURL string, err error) {
	normalizedType, ok := agent.NormalizeSourceType(sourceType)
	if !ok {
		return "", "", fmt.Errorf("source_type must be one of: %s, %s, %s, %s", agent.SourceGitHub, agent.SourceHTTP, agent.SourceFS, agent.SourceS3)
	}
	if normalizedType == agent.SourceGitHub {
		return normalizedType, metadataURL, nil
	}
	root, err := agent.NormalizeFeedLocation(normalizedType, metadataURL)
	if err != nil {
		return "", "", fmt.Errorf("metadata_url: %w", err)
	}
	if err := agent.ValidateFeedReleaseRef(defaultReleaseRef(releaseRef)); err != nil {
		return "", "", err
	}
	return normalizedType, root, nil
}

// resolveFeedAuth validates a feed credential for the resolved source type.
// http feeds take bearer/basic/header credentials and s3 feeds require sigv4;
// file shares use the Windows account Watcher runs as.
func resolveFeedAuth(sourceType string, auth agent.FeedAuth) (agent.FeedAuth, error) {
	normalized, err := agent.NormalizeFeedAuth(auth)
	if err != nil {
		return agent.FeedAuth{}, fmt.Errorf("feed_auth: %w", err)
	}
	switch {
	case sourceType == agent.SourceS3 && normalized.Type != agent.FeedAuthSigV4:
		return agent.FeedAuth{}, fmt.Errorf("s3 sources need feed_auth_type %q with an access key, secret key and region", agent.FeedAuthSigV4)
	case sourceType != agent.SourceS3 && normalized.Type == agent.FeedAuthSigV4:
		return agent.FeedAuth{}, fmt.Errorf("feed_auth_type %q is only supported for %s sources", agent.FeedAuthSigV4, agent.SourceS3)
	case normalized.Type != "" && sourceType != agent.SourceHTTP && sourceType != agent.SourceS3:
		return agent.FeedAuth{}, fmt.Errorf("feed credentials are only supported for %s and %s sources", agent.SourceHTTP, agent.SourceS3)
	}
	return normalized, nil
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
