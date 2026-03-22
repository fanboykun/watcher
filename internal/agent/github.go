package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultAPIBase = "https://api.github.com"

type VersionMetadata struct {
	Services map[string]ServiceMeta `json:"services"`
}

type ServiceMeta struct {
	Version     string `json:"version"`
	Artifact    string `json:"artifact"`
	ArtifactURL string `json:"artifact_url"`
	PublishedAt string `json:"published_at"`
}

// githubRelease is the subset of the GitHub releases API response we need
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	URL                string `json:"url"`                  // API download URL — works for private repos
	BrowserDownloadURL string `json:"browser_download_url"` // Direct download URL — for reference
}

type GitHubClient struct {
	token   string
	apiBase string // defaults to defaultAPIBase, overridable in tests
	client  *http.Client
	log     *Logger
}

func NewGitHubClient(token string, log *Logger) *GitHubClient {
	return &GitHubClient{
		token:   token,
		apiBase: defaultAPIBase,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
		log: log,
	}
}

// FetchMetadata downloads and parses version.json from a GitHub release.
//
// For PUBLIC repos:
//   Uses the direct releases/latest/download/version.json URL (no token needed)
//
// For PRIVATE repos:
//   Uses the GitHub API to find the asset, then downloads via the asset API URL.
//   Requires github_token in .env with repo scope.
//
// The metadata_url configuration should always be:
//   https://github.com/{owner}/{repo}/releases/latest/download/version.json
// This function handles routing to the correct method automatically.
func (g *GitHubClient) FetchMetadata(ctx context.Context, url string) (*VersionMetadata, error) {
	g.log.Debug("fetching metadata", "url", url)

	var data []byte
	var err error

	if g.token != "" && !strings.HasSuffix(url, "version.json") {
		// Native repo polling: use GitHub API to find latest release and build metadata
		return g.FetchMetadataFromRepo(ctx, url)
	}

	if g.token != "" {
		// Private repo with version.json: use GitHub API to resolve and download the asset
		owner, repo, err := ParseGitHubURL(url)
		if err != nil {
			return nil, fmt.Errorf("parse metadata URL: %w", err)
		}
		data, err = g.fetchAssetViaAPI(ctx, owner, repo, "version.json")
		if err != nil {
			return nil, fmt.Errorf("fetch version.json via API: %w", err)
		}
	} else {
		// Public repo: fetch directly (can be either version.json or repo URL)
		if !strings.HasSuffix(url, "version.json") {
			return g.FetchMetadataFromRepo(ctx, url)
		}
		data, err = g.fetchDirect(ctx, url)
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

// fetchAssetViaAPI uses the GitHub releases API to find and download a named asset.
// This is the correct method for private repo release assets.
func (g *GitHubClient) fetchAssetViaAPI(ctx context.Context, owner, repo, assetName string) ([]byte, error) {
	// Step 1: get latest release metadata to find the asset ID
	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.apiBase, owner, repo)
	g.log.Debug("fetching latest release via API", "url", apiURL)

	req, err := g.newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d -- check github_token has repo scope", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("HTTP 404 -- no releases found for %s/%s", owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}

	// Step 2: find the asset by name
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
		return nil, fmt.Errorf("asset %q not found in latest release (available: %v)", assetName, names)
	}

	// Step 3: download the asset via its API URL
	g.log.Debug("downloading asset via API", "asset", assetName, "url", asset.URL)

	req2, err := g.newRequest(ctx, http.MethodGet, asset.URL)
	if err != nil {
		return nil, err
	}
	// Required header to get the raw binary instead of JSON metadata
	req2.Header.Set("Accept", "application/octet-stream")
	req2.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp2, err := g.client.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("asset download returned HTTP %d", resp2.StatusCode)
	}

	return io.ReadAll(resp2.Body)
}

// fetchDirect downloads a URL directly (public repos only)
func (g *GitHubClient) fetchDirect(ctx context.Context, url string) ([]byte, error) {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("HTTP %d -- repo may be private, set GITHUB_TOKEN in .env", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("HTTP 404 -- confirm a release exists and version.json was uploaded as a release asset")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// DownloadArtifact downloads the release zip to destPath with retry.
// For private repos, uses the GitHub API asset URL from version.json's artifact_url.
func (g *GitHubClient) DownloadArtifact(ctx context.Context, artifactURL, destPath string, retries int) error {
	var lastErr error

	for attempt := 1; attempt <= retries; attempt++ {
		g.log.Info("downloading artifact", "url", artifactURL, "attempt", attempt)

		var err error
		if g.token != "" {
			// Private repo: artifact_url is a browser download URL, resolve via API
			owner, repo, assetName, err2 := parseArtifactURL(artifactURL)
			if err2 != nil {
				return fmt.Errorf("parse artifact URL: %w", err2)
			}
			err = g.downloadAssetViaAPI(ctx, owner, repo, assetName, destPath)
		} else {
			err = g.downloadDirect(ctx, artifactURL, destPath)
		}

		if err == nil {
			g.log.Info("artifact downloaded", "dest", destPath)
			return nil
		}

		lastErr = err
		g.log.Warn("download attempt failed", "attempt", attempt, "error", err)

		if attempt < retries {
			backoff := time.Duration(attempt*attempt) * time.Second
			g.log.Info("backing off", "seconds", backoff.Seconds())
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}

	return fmt.Errorf("download failed after %d attempts: %w", retries, lastErr)
}

// downloadAssetViaAPI finds the artifact by name in the matching release and downloads it
func (g *GitHubClient) downloadAssetViaAPI(ctx context.Context, owner, repo, assetName, destPath string) error {
	data, err := g.fetchAssetViaAPI(ctx, owner, repo, assetName)
	if err != nil {
		return err
	}

	tmp := destPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename artifact: %w", err)
	}
	return nil
}

// downloadDirect downloads a URL to a file (public repos)
func (g *GitHubClient) downloadDirect(ctx context.Context, url, destPath string) error {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmp := destPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	_, copyErr := io.Copy(f, resp.Body)
	f.Close()

	if copyErr != nil {
		os.Remove(tmp)
		return fmt.Errorf("write artifact: %w", copyErr)
	}

	if err := os.Rename(tmp, destPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename artifact: %w", err)
	}

	return nil
}

func (g *GitHubClient) newRequest(ctx context.Context, method, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// ParseGitHubURL extracts owner and repo from a GitHub URL.
// Supports: https://github.com/{owner}/{repo}
func ParseGitHubURL(rawURL string) (owner, repo string, err error) {
	trimmed := strings.TrimPrefix(rawURL, "https://github.com/")
	if trimmed == rawURL {
		return "", "", fmt.Errorf("unexpected URL format (expected https://github.com/...): %s", rawURL)
	}
	parts := strings.SplitN(trimmed, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot extract owner/repo from URL: %s", rawURL)
	}
	return parts[0], parts[1], nil
}

// parseArtifactURL extracts owner, repo, and asset filename from a GitHub release download URL.
// Supports: https://github.com/{owner}/{repo}/releases/download/{tag}/{filename}
func parseArtifactURL(rawURL string) (owner, repo, assetName string, err error) {
	trimmed := strings.TrimPrefix(rawURL, "https://github.com/")
	if trimmed == rawURL {
		return "", "", "", fmt.Errorf("unexpected URL format: %s", rawURL)
	}
	// owner/repo/releases/download/vX.Y.Z/filename.zip
	parts := strings.SplitN(trimmed, "/", 6)
	if len(parts) < 6 || parts[5] == "" {
		return "", "", "", fmt.Errorf("cannot extract asset name from URL: %s", rawURL)
	}
	return parts[0], parts[1], parts[5], nil
}

// InspectRepoResponse represents the payload returned for the UI to preview releases.
type InspectRepoResponse struct {
	LatestVersion string   `json:"latest_version"`
	PublishedAt   string   `json:"published_at"`
	Assets        []string `json:"assets"`
}// InspectRepository fetches the latest release from a GitHub repository to preview assets.
func (g *GitHubClient) InspectRepository(ctx context.Context, url string) (*InspectRepoResponse, error) {
	meta, err := g.FetchMetadataFromRepo(ctx, url)
	if err != nil {
		return nil, err
	}

	res := &InspectRepoResponse{}
	for _, svc := range meta.Services {
		res.LatestVersion = svc.Version
		res.PublishedAt = svc.PublishedAt
		res.Assets = append(res.Assets, svc.Artifact)
	}
	return res, nil
}

// FetchMetadataFromRepo builds a VersionMetadata object directly from GitHub release data.
func (g *GitHubClient) FetchMetadataFromRepo(ctx context.Context, repoURL string) (*VersionMetadata, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}

	apiURL := fmt.Sprintf("%s/repos/%s/%s/releases/latest", g.apiBase, owner, repo)
	req, err := g.newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases API returned HTTP %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release JSON: %w", err)
	}

	meta := &VersionMetadata{
		Services: make(map[string]ServiceMeta),
	}

	for _, asset := range release.Assets {
		// Map asset name to a "service". We strip common extensions for matching.
		name := asset.Name
		name = strings.TrimSuffix(name, ".zip")
		name = strings.TrimSuffix(name, ".exe")

		// Also try to strip version suffixes like -v1.2.3 or -1.2.3
		// We look for a hyphen followed by 'v' and a digit, or just a hyphen and a digit.
		serviceName := name
		if idx := strings.LastIndex(name, "-"); idx != -1 {
			suffix := name[idx+1:]
			if len(suffix) > 0 && (suffix[0] == 'v' || (suffix[0] >= '0' && suffix[0] <= '9')) {
				serviceName = name[:idx]
			}
		}

		meta.Services[serviceName] = ServiceMeta{
			Version:     release.TagName,
			Artifact:    asset.Name,
			ArtifactURL: asset.BrowserDownloadURL,
			PublishedAt: release.PublishedAt,
		}
	}

	return meta, nil
}

