package agent

import (
	"context"
	"strings"
)

// Release source types a watcher can poll. Watchers created before sources
// existed have an empty type and keep the GitHub behavior.
const (
	SourceGitHub = "github"
	SourceHTTP   = "http"
	SourceFS     = "fs" // local directory or SMB/UNC share
	SourceS3     = "s3" // private S3-compatible bucket, read over HTTP(S) with SigV4
)

// releaseSource resolves release manifests and fetches artifacts for one watcher.
// Manifests it returns carry absolute artifact URLs.
type releaseSource interface {
	FetchMetadata(ctx context.Context, releaseRef, serviceName string) (*VersionMetadata, error)
	DownloadArtifact(ctx context.Context, artifactURL, destPath string, retries int) error
}

// NormalizeSourceType returns the canonical source type, or false when it is unsupported.
func NormalizeSourceType(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", SourceGitHub:
		return SourceGitHub, true
	case SourceHTTP:
		return SourceHTTP, true
	case SourceFS:
		return SourceFS, true
	case SourceS3:
		return SourceS3, true
	}
	return "", false
}

// NormalizeFeedLocation validates the feed root for a non-GitHub source type.
// An s3 feed root is the bucket URL plus prefix, path-style or virtual-hosted.
func NormalizeFeedLocation(sourceType, raw string) (string, error) {
	if sourceType == SourceFS {
		return NormalizeFSFeedRoot(raw)
	}
	return NormalizeFeedRoot(raw)
}

// newFeedSource returns the reader for a non-GitHub source type. An s3 feed is
// an HTTP feed with a SigV4 credential. File share feeds authenticate as the
// Windows account Watcher runs as, so they take no credential.
func newFeedSource(sourceType, root string, auth FeedAuth, log *Logger) feedSource {
	if sourceType == SourceFS {
		return newFSFeedSource(root, log)
	}
	return newHTTPFeedSource(root, auth, log)
}

// IsGitHubSource reports whether GitHub-only features (deployment reporting,
// release catalog, repo inspection) apply to a watcher with this source type.
func IsGitHubSource(sourceType string) bool {
	normalized, _ := NormalizeSourceType(sourceType)
	return normalized == SourceGitHub
}

// githubSource adapts the GitHub client, which already understands repo URLs,
// version.json release assets and private-repo API downloads.
type githubSource struct {
	client      *GitHubClient
	metadataURL string
}

func (s githubSource) FetchMetadata(ctx context.Context, releaseRef, serviceName string) (*VersionMetadata, error) {
	return s.client.FetchServiceMetadataForRelease(ctx, s.metadataURL, releaseRef, serviceName)
}

func (s githubSource) DownloadArtifact(ctx context.Context, artifactURL, destPath string, retries int) error {
	return s.client.DownloadArtifact(ctx, artifactURL, destPath, retries)
}

// releaseSource returns the source this watcher polls. gh is still used for
// GitHub-backed watchers so they share pooled rate-limit state.
func (r *RepoWatcher) releaseSource(gh *GitHubClient) releaseSource {
	if sourceType, _ := NormalizeSourceType(r.wcfg.SourceType); sourceType != SourceGitHub {
		source := newFeedSource(sourceType, r.wcfg.MetadataURL, r.wcfg.FeedAuth, r.log)
		source.setPollCache(r.feedPoll)
		return source
	}
	return githubSource{client: gh, metadataURL: r.wcfg.MetadataURL}
}

// githubQuotaShortfall reports a token that cannot afford a deploy. Feed
// watchers spend no GitHub quota, so they are never held back.
func (r *RepoWatcher) githubQuotaShortfall(gh *GitHubClient) *RateLimitError {
	if !IsGitHubSource(r.wcfg.SourceType) {
		return nil
	}
	return gh.quotaShortfall(deployRequestBudget)
}
