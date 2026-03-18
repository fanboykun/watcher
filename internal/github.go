package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type VersionMetadata struct {
	Services map[string]ServiceMeta `json:"services"`
}

type ServiceMeta struct {
	Version     string `json:"version"`
	Artifact    string `json:"artifact"`
	ArtifactURL string `json:"artifact_url"`
	PublishedAt string `json:"published_at"`
}

type GitHubClient struct {
	token  string
	client *http.Client
	log    *Logger
}

func NewGitHubClient(token string, log *Logger) *GitHubClient {
	return &GitHubClient{
		token: token,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
		log: log,
	}
}

func (g *GitHubClient) FetchMetadata(ctx context.Context, url string) (*VersionMetadata, error) {
	g.log.Debug("fetching metadata", "url", url)

	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata fetch returned HTTP %d from %s", resp.StatusCode, url)
	}

	var meta VersionMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("decode metadata JSON: %w", err)
	}

	return &meta, nil
}

func (g *GitHubClient) DownloadArtifact(ctx context.Context, url, destPath string, retries int) error {
	var lastErr error

	for attempt := 1; attempt <= retries; attempt++ {
		g.log.Info("downloading artifact", "url", url, "attempt", attempt)

		err := g.downloadOnce(ctx, url, destPath)
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

func (g *GitHubClient) downloadOnce(ctx context.Context, url, destPath string) error {
	req, err := g.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")

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