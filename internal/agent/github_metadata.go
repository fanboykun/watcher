package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// FetchMetadata fetches metadata.
func (g *GitHubClient) FetchMetadata(ctx context.Context, url string) (*VersionMetadata, error) {
	return g.FetchMetadataForRelease(ctx, url, "latest")
}

// FetchServiceMetadataForRelease fetches service metadata for release.
func (g *GitHubClient) FetchServiceMetadataForRelease(ctx context.Context, rawURL, releaseRef, serviceName string) (*VersionMetadata, error) {
	releaseRef = normalizeReleaseRef(releaseRef)
	serviceName = strings.TrimSpace(serviceName)
	g.log.Debug("fetching service metadata", "url", rawURL, "release_ref", releaseRef, "service_name", serviceName)

	if strings.HasSuffix(rawURL, "version.json") {
		return g.FetchMetadataForRelease(ctx, rawURL, releaseRef)
	}

	return g.FetchMetadataFromRepoForServiceRelease(ctx, rawURL, releaseRef, serviceName)
}

// FetchMetadataForRelease fetches metadata for release.
func (g *GitHubClient) FetchMetadataForRelease(ctx context.Context, rawURL, releaseRef string) (*VersionMetadata, error) {
	releaseRef = normalizeReleaseRef(releaseRef)
	g.log.Debug("fetching metadata", "url", rawURL, "release_ref", releaseRef)

	if !strings.HasSuffix(rawURL, "version.json") {
		return g.FetchMetadataFromRepoForRelease(ctx, rawURL, releaseRef)
	}

	if releaseRef != "latest" {
		owner, repo, err := ParseGitHubURL(rawURL)
		if err != nil {
			return nil, fmt.Errorf("parse metadata URL: %w", err)
		}
		data, err := g.fetchNamedAssetFromRelease(ctx, owner, repo, releaseRef, "version.json")
		if err != nil {
			return nil, fmt.Errorf("fetch version.json for release %q: %w", releaseRef, err)
		}
		var meta VersionMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("decode version.json: %w", err)
		}
		return &meta, nil
	}

	return g.fetchLatestMetadata(ctx, rawURL)
}

// fetchLatestMetadata fetches latest metadata.
func (g *GitHubClient) fetchLatestMetadata(ctx context.Context, rawURL string) (*VersionMetadata, error) {
	g.log.Debug("fetching metadata", "url", rawURL)

	var data []byte
	var err error

	if g.token != "" {
		// Private repo with version.json: use GitHub API to resolve and download the asset
		owner, repo, err := ParseGitHubURL(rawURL)
		if err != nil {
			return nil, fmt.Errorf("parse metadata URL: %w", err)
		}
		data, err = g.fetchNamedAssetFromRelease(ctx, owner, repo, "latest", "version.json")
		if err != nil {
			return nil, fmt.Errorf("fetch version.json via API: %w", err)
		}
	} else {
		data, err = g.fetchDirect(ctx, rawURL)
		if err != nil {
			return nil, fmt.Errorf("fetch metadata directly: %w", err)
		}
	}

	var meta VersionMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("decode version.json: %w", err)
	}

	return &meta, nil
}

// FetchMetadataFromRepo builds metadata from a repository's latest release.
func (g *GitHubClient) FetchMetadataFromRepo(ctx context.Context, repoURL string) (*VersionMetadata, error) {
	return g.FetchMetadataFromRepoForRelease(ctx, repoURL, "latest")
}

// FetchMetadataFromRepoForRelease fetches metadata from repo for release.
func (g *GitHubClient) FetchMetadataFromRepoForRelease(ctx context.Context, repoURL, releaseRef string) (*VersionMetadata, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	release, err := g.fetchRelease(ctx, owner, repo, releaseRef)
	if err != nil {
		return nil, err
	}

	return buildMetadataFromRelease(release), nil
}

// FetchMetadataFromRepoForServiceRelease fetches metadata from repo for service release.
func (g *GitHubClient) FetchMetadataFromRepoForServiceRelease(ctx context.Context, repoURL, releaseRef, serviceName string) (*VersionMetadata, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	releaseRef = normalizeReleaseRef(releaseRef)
	serviceName = strings.TrimSpace(serviceName)
	if releaseRef != "latest" || serviceName == "" {
		return g.FetchMetadataFromRepoForRelease(ctx, repoURL, releaseRef)
	}

	release, err := g.fetchLatestReleaseForService(ctx, owner, repo, serviceName)
	if err != nil {
		return nil, err
	}
	return buildMetadataFromRelease(release), nil
}

// fetchAssetViaAPI uses the GitHub releases API to find and download a named asset.
// This is the correct method for private repo release assets.
