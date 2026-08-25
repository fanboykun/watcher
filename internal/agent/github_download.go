package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// fetchAssetViaAPI fetches asset via api.
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

// newRequest creates an authenticated GitHub API request bound to the supplied context.
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
