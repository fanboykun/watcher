package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var knownAssetSuffixTokens = map[string]struct{}{
	"windows": {}, "linux": {}, "darwin": {}, "macos": {},
	"amd64": {}, "x64": {}, "386": {}, "x86": {}, "arm64": {}, "arm": {},
}

const defaultAPIBase = "https://api.github.com"
const githubDeploymentStatusDescriptionLimit = 140

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
//
//	Uses the direct releases/latest/download/version.json URL (no token needed)
//
// For PRIVATE repos:
//
//	Uses the GitHub API to find the asset, then downloads via the asset API URL.
//	Requires github_token in .env with repo scope.
//
// The metadata_url configuration should always be:
//
//	https://github.com/{owner}/{repo}/releases/latest/download/version.json
//
// This function handles routing to the correct method automatically.
func (g *GitHubClient) FetchMetadata(ctx context.Context, url string) (*VersionMetadata, error) {
	return g.FetchMetadataForRelease(ctx, url, "latest")
}

func (g *GitHubClient) FetchServiceMetadataForRelease(ctx context.Context, rawURL, releaseRef, serviceName string) (*VersionMetadata, error) {
	releaseRef = normalizeReleaseRef(releaseRef)
	serviceName = strings.TrimSpace(serviceName)
	g.log.Debug("fetching service metadata", "url", rawURL, "release_ref", releaseRef, "service_name", serviceName)

	if strings.HasSuffix(rawURL, "version.json") {
		return g.FetchMetadataForRelease(ctx, rawURL, releaseRef)
	}

	return g.FetchMetadataFromRepoForServiceRelease(ctx, rawURL, releaseRef, serviceName)
}

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
func (g *GitHubClient) fetchAssetViaAPI(ctx context.Context, owner, repo, assetName string) ([]byte, error) {
	return g.fetchNamedAssetFromRelease(ctx, owner, repo, "latest", assetName)
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
			owner, repo, tag, assetName, err2 := parseReleaseDownloadURL(artifactURL)
			if err2 != nil {
				return fmt.Errorf("parse artifact URL: %w", err2)
			}
			err = g.downloadAssetViaAPI(ctx, owner, repo, tag, assetName, destPath)
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
func (g *GitHubClient) downloadAssetViaAPI(ctx context.Context, owner, repo, releaseRef, assetName, destPath string) error {
	data, err := g.fetchNamedAssetFromRelease(ctx, owner, repo, releaseRef, assetName)
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
// Supports:
// - https://github.com/{owner}/{repo}
// - http://github.com/{owner}/{repo}
// - github.com/{owner}/{repo}
// - git@github.com:{owner}/{repo}.git
// Any extra path segments are ignored (e.g. /tree/main, /releases/latest...).
func ParseGitHubURL(rawURL string) (owner, repo string, err error) {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return "", "", fmt.Errorf("empty GitHub URL")
	}
	if strings.HasPrefix(s, "git@github.com:") {
		s = "https://github.com/" + strings.TrimPrefix(s, "git@github.com:")
	}
	if strings.HasPrefix(s, "github.com/") {
		s = "https://" + s
	}
	if !strings.Contains(s, "://") {
		return "", "", fmt.Errorf("unexpected URL format (expected github.com/owner/repo): %s", rawURL)
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub URL: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", "", fmt.Errorf("unsupported host %q (expected github.com)", u.Host)
	}

	path := strings.Trim(u.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("cannot extract owner/repo from URL: %s", rawURL)
	}
	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", fmt.Errorf("cannot extract owner/repo from URL: %s", rawURL)
	}
	return owner, repo, nil
}

// parseArtifactURL extracts owner, repo, and asset filename from a GitHub release download URL.
// Supports: https://github.com/{owner}/{repo}/releases/download/{tag}/{filename}
func parseArtifactURL(rawURL string) (owner, repo, assetName string, err error) {
	owner, repo, _, assetName, err = parseReleaseDownloadURL(rawURL)
	return owner, repo, assetName, err
}

// parseReleaseDownloadURL extracts owner, repo, release tag, and asset filename
// from a GitHub release download URL.
// Supports: https://github.com/{owner}/{repo}/releases/download/{tag}/{filename}
func parseReleaseDownloadURL(rawURL string) (owner, repo, tag, assetName string, err error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid URL: %w", err)
	}
	if strings.ToLower(strings.TrimSpace(u.Scheme)) != "https" {
		return "", "", "", "", fmt.Errorf("unexpected URL format: %s", rawURL)
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host != "github.com" && host != "www.github.com" {
		return "", "", "", "", fmt.Errorf("unexpected URL format: %s", rawURL)
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 6 {
		return "", "", "", "", fmt.Errorf("cannot extract asset name from URL: %s", rawURL)
	}
	if parts[2] != "releases" || parts[3] != "download" {
		return "", "", "", "", fmt.Errorf("unexpected release download URL format: %s", rawURL)
	}

	owner = parts[0]
	repo = parts[1]

	tagEscaped := strings.Join(parts[4:len(parts)-1], "/")
	if tagEscaped == "" {
		return "", "", "", "", fmt.Errorf("unexpected release download URL format: %s", rawURL)
	}
	tag, err = url.PathUnescape(tagEscaped)
	if err != nil {
		return "", "", "", "", fmt.Errorf("decode release tag: %w", err)
	}

	assetName, err = url.PathUnescape(parts[len(parts)-1])
	if err != nil {
		return "", "", "", "", fmt.Errorf("decode asset name: %w", err)
	}
	if assetName == "" {
		return "", "", "", "", fmt.Errorf("cannot extract asset name from URL: %s", rawURL)
	}

	return owner, repo, tag, assetName, nil
}

func deriveServiceNameFromAsset(assetName string) string {
	name := strings.TrimSpace(assetName)

	for _, ext := range []string{".tar.gz", ".tar.xz", ".tar.bz2", ".tgz", ".zip", ".exe", ".msi"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			name = name[:len(name)-len(ext)]
			break
		}
	}

	parts := strings.Split(name, "-")
	for len(parts) > 1 {
		last := parts[len(parts)-1]
		if isVersionLikeToken(last) || isKnownAssetSuffixToken(last) {
			parts = parts[:len(parts)-1]
			continue
		}
		break
	}

	derived := strings.Join(parts, "-")
	if strings.TrimSpace(derived) == "" {
		return name
	}
	return derived
}

func shouldIgnoreAssetForServiceDiscovery(assetName string) bool {
	name := strings.ToLower(strings.TrimSpace(assetName))
	return name == "version.json" || strings.HasSuffix(name, ".version.json")
}

func buildMetadataFromRelease(release *githubRelease) *VersionMetadata {
	meta := &VersionMetadata{
		Services: make(map[string]ServiceMeta),
	}

	for _, asset := range release.Assets {
		if shouldIgnoreAssetForServiceDiscovery(asset.Name) {
			continue
		}
		serviceName := deriveServiceNameFromAsset(asset.Name)
		if strings.TrimSpace(serviceName) == "" {
			continue
		}

		meta.Services[serviceName] = ServiceMeta{
			Version:     release.TagName,
			Artifact:    asset.Name,
			ArtifactURL: asset.BrowserDownloadURL,
			PublishedAt: release.PublishedAt,
		}
	}

	return meta
}

func isKnownAssetSuffixToken(token string) bool {
	_, ok := knownAssetSuffixTokens[strings.ToLower(strings.TrimSpace(token))]
	return ok
}

func isVersionLikeToken(token string) bool {
	s := strings.TrimSpace(strings.ToLower(token))
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "v") {
		s = s[1:]
	}
	if s == "" {
		return false
	}

	hasDigit := false
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r == '.', r == '_':
		default:
			return false
		}
	}
	return hasDigit
}

// InspectRepoResponse represents the payload returned for the UI to preview releases.
type InspectRepoResponse struct {
	LatestVersion string   `json:"latest_version"`
	PublishedAt   string   `json:"published_at"`
	Assets        []string `json:"assets"`
}

// InspectRepository fetches the latest release from a GitHub repository to preview assets.
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

// ── GitHub Deployment API ─────────────────────────────────────────────

// CreateDeployment creates a new deployment on GitHub for the given repo/ref.
// Returns the deployment ID which is needed to update the status later.
func (g *GitHubClient) CreateDeployment(ctx context.Context, owner, repo, ref, environment, description string) (int64, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/deployments", g.apiBase, owner, repo)

	body := map[string]any{
		"ref":               ref,
		"environment":       environment,
		"description":       description,
		"auto_merge":        false,
		"required_contexts": []string{}, // skip status checks — we're deploying ourselves
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := g.newRequest(ctx, http.MethodPost, apiURL)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(string(bodyJSON)))

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("create deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		hint := githubDeployHint(resp.StatusCode)
		return 0, fmt.Errorf("create deployment returned HTTP %d: %s%s", resp.StatusCode, string(respBody), hint)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode deployment response: %w", err)
	}

	g.log.Info("GitHub deployment created", "deployment_id", result.ID, "ref", ref)
	return result.ID, nil
}

// UpdateDeploymentStatus updates the status of a GitHub deployment.
// Valid states: "pending", "in_progress", "success", "failure", "error", "inactive"
func (g *GitHubClient) UpdateDeploymentStatus(ctx context.Context, owner, repo string, deploymentID int64, state, logURL, description string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/deployments/%d/statuses", g.apiBase, owner, repo, deploymentID)

	body := map[string]string{
		"state":       state,
		"description": limitGitHubDeploymentDescription(description, githubDeploymentStatusDescriptionLimit),
	}
	if logURL != "" {
		body["log_url"] = logURL
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := g.newRequest(ctx, http.MethodPost, apiURL)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(string(bodyJSON)))

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		hint := githubDeployHint(resp.StatusCode)
		return fmt.Errorf("update deployment status returned HTTP %d: %s%s", resp.StatusCode, string(respBody), hint)
	}

	g.log.Debug("deployment status updated", "deployment_id", deploymentID, "state", state)
	return nil
}

func limitGitHubDeploymentDescription(description string, maxRunes int) string {
	description = strings.TrimSpace(description)
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(description)
	if len(runes) <= maxRunes {
		return description
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func githubDeployHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return " (hint: check GITHUB_TOKEN validity and scopes)"
	case http.StatusForbidden:
		return " (hint: token may lack Deployments write permission)"
	case http.StatusNotFound:
		return " (hint: owner/repo/ref may be wrong, or token cannot access the repo)"
	case http.StatusUnprocessableEntity:
		return " (hint: ref may not exist in repo, environment/log_url may be invalid, or branch protection blocks deployments)"
	default:
		return ""
	}
}

// FetchLatestRelease fetches the latest release info from a GitHub repository.
// Used for self-update checks.
func (g *GitHubClient) FetchLatestRelease(ctx context.Context, repoURL string) (*githubRelease, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}
	return g.fetchRelease(ctx, owner, repo, "latest")
}

func normalizeReleaseRef(releaseRef string) string {
	releaseRef = strings.TrimSpace(releaseRef)
	if releaseRef == "" {
		return "latest"
	}
	return releaseRef
}

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
