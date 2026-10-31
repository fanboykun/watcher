package agent

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fanboykun/watcher/internal/database"
)

const feedCatalogPageSize = 30

// FeedCatalog serves a feed's releases.json through the same release-catalog
// contract as GitHub, so downloading, configuring and deploying a chosen
// version work the same way for feed watchers.
type FeedCatalog struct {
	source      feedSource
	serviceName string
}

// NewFeedCatalog returns the catalog for a non-GitHub watcher.
func NewFeedCatalog(watcher *database.Watcher, log *Logger) (*FeedCatalog, error) {
	sourceType, ok := NormalizeSourceType(watcher.SourceType)
	if !ok || sourceType == SourceGitHub {
		return nil, fmt.Errorf("watcher %d does not use a release feed", watcher.ID)
	}
	auth := FeedAuth{Type: watcher.FeedAuthType, Username: watcher.FeedAuthUsername, Header: watcher.FeedAuthHeader, Region: watcher.FeedAuthRegion, Secret: watcher.FeedAuthSecret}
	return &FeedCatalog{source: newFeedSource(sourceType, watcher.MetadataURL, auth, log), serviceName: watcher.ServiceName}, nil
}

// feedID derives a stable positive ID from release identity. GitHub IDs are
// assigned by GitHub; feeds have none, and IDs must survive new releases being
// prepended to the index. 53 bits keep them exact as JavaScript numbers.
func feedID(parts ...string) int64 {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return max(int64(binary.BigEndian.Uint64(sum[:8])>>11), 1)
}

// catalogReleases lists index releases that publish this watcher's service,
// as catalog releases with that service's artifact as their only asset.
func (f *FeedCatalog) catalogReleases(ctx context.Context) ([]CatalogRelease, map[int64]ServiceMeta, error) {
	index, err := f.source.fetchIndex(ctx)
	if err != nil {
		return nil, nil, err
	}
	releases := make([]CatalogRelease, 0, len(index))
	services := make(map[int64]ServiceMeta, len(index))
	for _, entry := range index {
		svc, ok := entry.Services[f.serviceName]
		if !ok || strings.TrimSpace(entry.Tag) == "" || strings.TrimSpace(svc.ArtifactURL) == "" {
			continue
		}
		artifact := svc.Artifact
		if artifact == "" {
			artifact = filepath.Base(filepath.FromSlash(svc.ArtifactURL))
		}
		asset := CatalogAsset{ID: feedID(entry.Tag, f.serviceName, artifact), Name: artifact, Size: svc.Size, BrowserDownloadURL: svc.ArtifactURL}
		if sum, err := normalizeSHA256(svc.SHA256); err == nil && sum != "" {
			asset.Digest = "sha256:" + sum
		}
		releases = append(releases, CatalogRelease{
			ID: feedID(entry.Tag), Name: entry.Name, Body: entry.Notes, TagName: entry.Tag,
			Prerelease: entry.Prerelease, PublishedAt: entry.PublishedAt, Assets: []CatalogAsset{asset},
		})
		services[asset.ID] = svc
	}
	return releases, services, nil
}

// ListReleaseCatalog pages through releases.json, newest first as published.
func (f *FeedCatalog) ListReleaseCatalog(ctx context.Context, _ string, page int) (*ReleaseCatalog, error) {
	releases, _, err := f.catalogReleases(ctx)
	if err != nil {
		return nil, err
	}
	page = max(page, 1)
	start := min((page-1)*feedCatalogPageSize, len(releases))
	end := min(start+feedCatalogPageSize, len(releases))
	return &ReleaseCatalog{Releases: releases[start:end], Page: page, HasNext: end < len(releases)}, nil
}

// ResolveCatalogAsset finds a release and its artifact by catalog IDs and
// returns the version the service's manifest entry declares.
func (f *FeedCatalog) ResolveCatalogAsset(ctx context.Context, _ *database.Watcher, releaseID, assetID int64) (*CatalogRelease, *CatalogAsset, string, error) {
	releases, services, err := f.catalogReleases(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	for i := range releases {
		release := &releases[i]
		if release.ID != releaseID {
			continue
		}
		asset := &release.Assets[0]
		if asset.ID != assetID {
			return nil, nil, "", fmt.Errorf("asset does not belong to this release")
		}
		if !strings.HasSuffix(strings.ToLower(asset.Name), ".zip") {
			return nil, nil, "", fmt.Errorf("deployment requires a ZIP release asset")
		}
		if _, err := normalizeSHA256(services[asset.ID].SHA256); err != nil {
			return nil, nil, "", err
		}
		version := services[asset.ID].Version
		if strings.TrimSpace(version) == "" {
			version = release.TagName
		}
		return release, asset, version, nil
	}
	return nil, nil, "", fmt.Errorf("release is not in the feed's %s", feedIndexName)
}

// DownloadCatalogAsset stages the artifact at path, verified against its
// manifest checksum when the feed publishes one.
func (f *FeedCatalog) DownloadCatalogAsset(ctx context.Context, _ string, assetID int64, path, digest string) (string, error) {
	_, services, err := f.catalogReleases(ctx)
	if err != nil {
		return "", err
	}
	svc, ok := services[assetID]
	if !ok {
		return "", fmt.Errorf("release artifact is no longer in the feed's %s", feedIndexName)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "artifact-*.tmp")
	if err != nil {
		return "", err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := f.source.DownloadArtifact(ctx, svc.ArtifactURL, tmp.Name(), 1); err != nil {
		return "", err
	}
	return finishCatalogDownload(ctx, tmp.Name(), path, digest)
}
