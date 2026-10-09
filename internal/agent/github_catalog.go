package agent

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
)

// CatalogRelease describes a GitHub release and its published deployment assets.
type CatalogRelease = githubRelease
type CatalogAsset = githubAsset

type ReleaseCatalog struct {
	Releases []CatalogRelease `json:"releases"`
	Page     int              `json:"page"`
	HasNext  bool             `json:"has_next"`
}

// ListReleaseCatalog paginates the repository's releases, including prereleases.
func (g *GitHubClient) ListReleaseCatalog(ctx context.Context, repoURL string, page int) (*ReleaseCatalog, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	req, err := g.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/releases?per_page=30&page=%d", g.apiBase, owner, repo, page))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub releases returned HTTP %d; check repository access and token permissions", resp.StatusCode)
	}
	result := &ReleaseCatalog{Releases: []CatalogRelease{}, Page: page, HasNext: strings.Contains(resp.Header.Get("Link"), `rel="next"`)}
	if err := json.NewDecoder(resp.Body).Decode(&result.Releases); err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveCatalogAsset resolves client-provided IDs against the watcher's repository.
// version.json repositories must use the artifact declared for this watcher service.
func (g *GitHubClient) ResolveCatalogAsset(ctx context.Context, watcher *database.Watcher, releaseID, assetID int64) (*CatalogRelease, *githubAsset, string, error) {
	owner, repo, err := ParseGitHubURL(watcher.MetadataURL)
	if err != nil {
		return nil, nil, "", err
	}
	req, err := g.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/releases/%d", g.apiBase, owner, repo, releaseID))
	if err != nil {
		return nil, nil, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, "", fmt.Errorf("GitHub release returned HTTP %d", resp.StatusCode)
	}
	var release CatalogRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, nil, "", err
	}
	if release.ID != releaseID || release.Draft || strings.TrimSpace(release.TagName) == "" {
		return nil, nil, "", fmt.Errorf("release is not published")
	}
	for i := range release.Assets {
		asset := &release.Assets[i]
		if asset.ID != assetID {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(asset.Name), ".zip") {
			return nil, nil, "", fmt.Errorf("deployment requires a ZIP release asset")
		}
		version := release.TagName
		if strings.HasSuffix(strings.ToLower(watcher.MetadataURL), "version.json") {
			meta, err := g.FetchMetadataForRelease(ctx, watcher.MetadataURL, release.TagName)
			if err != nil {
				return nil, nil, "", err
			}
			svc, ok := meta.Services[watcher.ServiceName]
			if !ok || strings.TrimSpace(svc.Version) == "" || (svc.Artifact != asset.Name && (svc.ArtifactURL == "" || svc.ArtifactURL != asset.BrowserDownloadURL)) {
				return nil, nil, "", fmt.Errorf("selected asset is not the artifact declared for service %q in version.json", watcher.ServiceName)
			}
			version = svc.Version
		}
		return &release, asset, version, nil
	}
	return nil, nil, "", fmt.Errorf("asset does not belong to this release")
}

// DownloadCatalogAsset streams through the repository asset API, including private assets.
func (g *GitHubClient) DownloadCatalogAsset(ctx context.Context, repoURL string, assetID int64, path, digest string) (string, error) {
	owner, repo, err := ParseGitHubURL(repoURL)
	if err != nil {
		return "", err
	}
	req, err := g.newRequest(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", g.apiBase, owner, repo, assetID))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	downloadClient := *g.client
	downloadClient.Timeout = 10 * time.Minute
	resp, err := downloadClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download asset returned HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "artifact-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, hash), resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if strings.HasPrefix(digest, "sha256:") && !strings.EqualFold(strings.TrimPrefix(digest, "sha256:"), sum) {
		return "", fmt.Errorf("asset SHA-256 does not match GitHub digest")
	}
	archive, err := zip.OpenReader(f.Name())
	if err != nil {
		return "", fmt.Errorf("invalid ZIP asset: %w", err)
	}
	err = archive.Close()
	if err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return sum, nil
}

func CatalogArtifactPath(artifact *database.CatalogArtifact) string {
	return filepath.Join(artifact.InstallDir, "downloads", "catalog", fmt.Sprintf("%d", artifact.ID), "artifact.zip")
}

// ValidateCatalogArtifact detects missing or changed files before service operations.
func ValidateCatalogArtifact(watcher *database.Watcher, artifact *database.CatalogArtifact) error {
	owner, repo, err := ParseGitHubURL(watcher.MetadataURL)
	if err != nil {
		return err
	}
	if artifact.ServiceName != watcher.ServiceName || artifact.WatcherID != watcher.ID || artifact.Repository != owner+"/"+repo || filepath.Clean(artifact.InstallDir) != filepath.Clean(watcher.InstallDir) || artifact.DownloadedAt == nil {
		return fmt.Errorf("download belongs to a different watcher configuration; download again")
	}
	f, err := os.Open(CatalogArtifactPath(artifact))
	if err != nil {
		return fmt.Errorf("staged artifact unavailable: %w", err)
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return fmt.Errorf("staged artifact checksum changed; download again")
	}
	return nil
}

// RunCatalog deploys exactly the approved local artifact without resolving "latest".
// It runs on the existing watcher goroutine, so deployments remain serialized.
func (r *RepoWatcher) RunCatalog(ctx context.Context, catalogID uint) error {
	var watcher database.Watcher
	if err := r.db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, r.watcherID).Error; err != nil {
		return err
	}
	if watcher.PendingCatalogID != catalogID || watcher.ApprovedVersion == "" {
		return fmt.Errorf("catalog approval changed; deployment canceled")
	}
	var artifact database.CatalogArtifact
	if err := r.db.Where("id = ? AND watcher_id = ?", catalogID, watcher.ID).First(&artifact).Error; err != nil {
		return err
	}
	cycle := NewRepoWatcher(&watcher, r.db, r.global, r.log, r.state.events, r.webhooks)
	// Future scheduled cycles must use the environment committed (or restored) by this deployment.
	defer func() {
		var updated database.Watcher
		if err := r.db.Preload("Services").Preload("Services.ConfigFiles").First(&updated, r.watcherID).Error; err != nil {
			r.log.Warn("catalog: failed to refresh runtime services", "error", err)
			return
		}
		r.wcfg.Services = WatcherConfigFromDB(&updated).Services
	}()
	trace := TraceFromContext(ctx)
	cycle.log = cycle.log.WithTrace(trace)
	cycle.state.log, cycle.state.trace = cycle.log, trace
	cycle.state.catalogID = catalogID
	cycle.deployer.log, cycle.deployer.logFn = cycle.log, cycle.state.AppendDeployLog
	cycle.cachedArtifact = CatalogArtifactPath(&artifact)
	if watcher.ApprovedVersion != artifact.Version || watcher.PendingVersion != artifact.Version {
		return fmt.Errorf("catalog version does not match approval")
	}
	if err := ValidateCatalogArtifact(&watcher, &artifact); err != nil {
		cycle.recordDeployFailure(err, artifact.Version, watcher.CurrentVersion)
		_ = r.db.Model(&database.Watcher{}).Where("id = ? AND pending_catalog_id = ?", watcher.ID, catalogID).UpdateColumn("approved_version", "").Error
		return err
	}
	cycle.log.Info("catalog deployment started", "catalog_id", catalogID, "version", artifact.Version, "asset", artifact.AssetName)
	gh := NewGitHubClient(cycle.resolveGitHubToken(), cycle.log)
	err := cycle.deploy(ctx, gh, ServiceMeta{Version: artifact.Version, Artifact: artifact.AssetName, ArtifactURL: artifact.ArtifactURL}, artifact.Version, watcher.CurrentVersion)
	if err != nil {
		cycle.recordDeployFailure(err, artifact.Version, watcher.CurrentVersion)
		// Retain the exact artifact for retry, but require fresh operator approval.
		_ = r.db.Model(&database.Watcher{}).Where("id = ? AND pending_catalog_id = ?", watcher.ID, catalogID).UpdateColumn("approved_version", "").Error
		return err
	}
	cycle.log.Info("catalog deployment completed", "catalog_id", catalogID, "version", artifact.Version)
	return nil
}
