package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// FetchLatestRelease fetches latest release.
func (g *GitHubClient) FetchLatestRelease(ctx context.Context, repoURL string) (*githubRelease, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}
	return g.fetchRelease(ctx, owner, repo, "latest")
}

// normalizeReleaseRef normalizes release ref.
func normalizeReleaseRef(releaseRef string) string {
	releaseRef = strings.TrimSpace(releaseRef)
	if releaseRef == "" {
		return "latest"
	}
	return releaseRef
}

// fetchLatestReleaseForService fetches latest release for service.
func (g *GitHubClient) fetchLatestReleaseForService(ctx context.Context, owner, repo, serviceName string) (*githubRelease, error) {
	latest, err := g.fetchRelease(ctx, owner, repo, "latest")
	if err != nil {
		return nil, err
	}
	latestMeta := buildMetadataFromRelease(latest)
	if _, ok := latestMeta.Services[serviceName]; ok {
		return latest, nil
	}

	releases, err := g.fetchReleases(ctx, owner, repo, 30)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		meta := buildMetadataFromRelease(&releases[i])
		if _, ok := meta.Services[serviceName]; ok {
			return &releases[i], nil
		}
	}

	available := make([]string, 0, len(latestMeta.Services))
	for name := range latestMeta.Services {
		available = append(available, name)
	}
	return nil, fmt.Errorf("service %q not found in repo releases for %s/%s (latest available: %v)", serviceName, owner, repo, available)
}

// fetchRelease fetches release.
func (g *GitHubClient) fetchRelease(ctx context.Context, owner, repo, releaseRef string) (*githubRelease, error) {
	releaseRef = normalizeReleaseRef(releaseRef)

	var apiURL string
	if releaseRef == "latest" {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.apiBase, owner, repo)
	} else {
		apiURL = fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", g.apiBase, owner, repo, url.PathEscape(releaseRef))
	}

	req, err := g.newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release %q: %w", releaseRef, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d -- check github_token has repo scope", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		if releaseRef == "latest" {
			return nil, fmt.Errorf("HTTP 404 -- no releases found for %s/%s", owner, repo)
		}
		return nil, fmt.Errorf("HTTP 404 -- release tag %q not found for %s/%s", releaseRef, owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}

	return &release, nil
}

// fetchReleases fetches releases.
func (g *GitHubClient) fetchReleases(ctx context.Context, owner, repo string, perPage int) ([]githubRelease, error) {
	if perPage <= 0 {
		perPage = 30
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=%d", g.apiBase, owner, repo, perPage)
	req, err := g.newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d -- check github_token has repo scope", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list releases returned HTTP %d", resp.StatusCode)
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decode releases JSON: %w", err)
	}
	return releases, nil
}

// fetchNamedAssetFromRelease fetches named asset from release.
func (g *GitHubClient) fetchNamedAssetFromRelease(ctx context.Context, owner, repo, releaseRef, assetName string) ([]byte, error) {
	release, err := g.fetchRelease(ctx, owner, repo, releaseRef)
	if err != nil {
		return nil, err
	}

	var asset *githubAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		names := make([]string, len(release.Assets))
		for i, a := range release.Assets {
			names[i] = a.Name
		}
		return nil, fmt.Errorf("asset %q not found in release %q (available: %v)", assetName, release.TagName, names)
	}

	if g.token == "" {
		return g.fetchDirect(ctx, asset.BrowserDownloadURL)
	}

	g.log.Debug("downloading asset via API", "asset", assetName, "url", asset.URL, "release_ref", releaseRef)

	req, err := g.newRequest(ctx, http.MethodGet, asset.URL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
