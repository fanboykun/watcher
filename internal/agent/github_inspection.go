package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type InspectRepoResponse struct {
	Source        string                          `json:"source"`
	RepoURL       string                          `json:"repo_url"`
	MetadataURL   string                          `json:"metadata_url"`
	LatestVersion string                          `json:"latest_version"`
	PublishedAt   string                          `json:"published_at"`
	Assets        []string                        `json:"assets"`
	Services      map[string]InspectServiceTarget `json:"services"`
}

type InspectServiceTarget struct {
	Name               string `json:"name"`
	Version            string `json:"version"`
	Artifact           string `json:"artifact"`
	ArtifactURL        string `json:"artifact_url"`
	PublishedAt        string `json:"published_at"`
	AppKind            string `json:"app_kind"`
	WindowsServiceName string `json:"windows_service_name"`
	BinaryName         string `json:"binary_name"`
	StartArguments     string `json:"start_arguments"`
	EnvFile            string `json:"env_file"`
	HealthCheckURL     string `json:"health_check_url"`
	IISAppPool         string `json:"iis_app_pool"`
	IISSiteName        string `json:"iis_site_name"`
	IISManagedRuntime  string `json:"iis_managed_runtime"`
	PublicURL          string `json:"public_url"`
}

// InspectRepository fetches release metadata for the UI. It prefers a
// version.json manifest and falls back to repo asset discovery only when the
// source was a repo URL and no manifest exists.
func (g *GitHubClient) InspectRepository(ctx context.Context, rawURL, releaseRef string) (*InspectRepoResponse, error) {
	releaseRef = normalizeReleaseRef(releaseRef)
	if strings.HasSuffix(strings.TrimSpace(rawURL), "version.json") {
		meta, err := g.FetchMetadataForRelease(ctx, rawURL, releaseRef)
		if err != nil {
			return nil, err
		}
		return buildInspectResponse(rawURL, releaseRef, meta, "manifest"), nil
	}

	meta, err := g.FetchManifestMetadataFromRepoRelease(ctx, rawURL, releaseRef)
	if err == nil {
		return buildInspectResponse(rawURL, releaseRef, meta, "manifest"), nil
	}

	meta, err = g.FetchMetadataFromRepoForRelease(ctx, rawURL, releaseRef)
	if err != nil {
		return nil, err
	}
	return buildInspectResponse(rawURL, releaseRef, meta, "repo_assets"), nil
}

// FetchManifestMetadataFromRepoRelease fetches manifest metadata from repo release.
func (g *GitHubClient) FetchManifestMetadataFromRepoRelease(ctx context.Context, repoURL, releaseRef string) (*VersionMetadata, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}
	data, err := g.fetchNamedAssetFromRelease(ctx, owner, repo, normalizeReleaseRef(releaseRef), "version.json")
	if err != nil {
		return nil, err
	}
	var meta VersionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("decode version.json: %w", err)
	}
	return &meta, nil
}

// buildInspectResponse builds the repository-inspection response from resolved metadata.
func buildInspectResponse(rawURL, releaseRef string, meta *VersionMetadata, source string) *InspectRepoResponse {
	owner, repo, _ := ParseGitHubURL(rawURL)
	repoURL := ""
	if owner != "" && repo != "" {
		repoURL = fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	}

	res := &InspectRepoResponse{
		Source:      source,
		RepoURL:     repoURL,
		MetadataURL: manifestURLForInspect(rawURL, releaseRef),
		Services:    make(map[string]InspectServiceTarget, len(meta.Services)),
	}
	for name, svc := range meta.Services {
		if res.LatestVersion == "" {
			res.LatestVersion = svc.Version
		}
		if res.PublishedAt == "" {
			res.PublishedAt = svc.PublishedAt
		}
		res.Assets = append(res.Assets, svc.Artifact)
		res.Services[name] = InspectServiceTarget{
			Name:               name,
			Version:            svc.Version,
			Artifact:           svc.Artifact,
			ArtifactURL:        svc.ArtifactURL,
			PublishedAt:        svc.PublishedAt,
			AppKind:            svc.AppKind,
			WindowsServiceName: svc.WindowsServiceName,
			BinaryName:         svc.BinaryName,
			StartArguments:     svc.StartArguments,
			EnvFile:            svc.EnvFile,
			HealthCheckURL:     svc.HealthCheckURL,
			IISAppPool:         svc.IISAppPool,
			IISSiteName:        svc.IISSiteName,
			IISManagedRuntime:  svc.IISManagedRuntime,
			PublicURL:          svc.PublicURL,
		}
	}
	return res
}

// manifestURLForInspect builds the manifest URL used by repository inspection.
func manifestURLForInspect(rawURL, releaseRef string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasSuffix(rawURL, "version.json") {
		return rawURL
	}
	owner, repo, err := ParseGitHubURL(rawURL)
	if err != nil {
		return ""
	}
	releaseRef = normalizeReleaseRef(releaseRef)
	if releaseRef == "latest" {
		return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/version.json", owner, repo)
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/version.json", owner, repo, releaseRef)
}

// ── GitHub Deployment API ─────────────────────────────────────────────

// CreateDeployment creates a new deployment on GitHub for the given repo/ref.
// Returns the deployment ID which is needed to update the status later.
