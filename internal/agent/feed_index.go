package agent

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// feedIndexName lists a feed's releases for pinning and the release catalog.
const feedIndexName = "releases.json"

// feedIndex is releases.json. Releases are listed newest first; each carries
// the same service entries as its releases/<tag>/version.json so the catalog
// is one request, and monorepo feeds can list releases of different services.
type feedIndex struct {
	Releases []feedRelease `json:"releases"`
}

type feedRelease struct {
	Tag         string                 `json:"tag"` // directory under releases/; also the pinned release_ref
	Name        string                 `json:"name,omitempty"`
	PublishedAt string                 `json:"published_at,omitempty"`
	Prerelease  bool                   `json:"prerelease,omitempty"`
	Notes       string                 `json:"notes,omitempty"`
	Services    map[string]ServiceMeta `json:"services"`
}

// feedSource is a release source backed by an internal feed's directory tree.
type feedSource interface {
	releaseSource
	// fetchIndex reads releases.json with artifact locations resolved.
	fetchIndex(ctx context.Context) ([]feedRelease, error)
	// setPollCache lets a watcher's poll loop revalidate instead of refetching.
	setPollCache(cache *feedPollCache)
}

// feedManifestPath maps a release ref to its manifest below the feed root.
func feedManifestPath(releaseRef string) (string, error) {
	ref := normalizeReleaseRef(releaseRef)
	if ref == "latest" {
		return feedManifestName, nil
	}
	if err := ValidateFeedReleaseRef(ref); err != nil {
		return "", err
	}
	return "releases/" + ref + "/" + feedManifestName, nil
}

// ValidateFeedReleaseRef rejects refs that cannot safely name a directory
// below releases/. Slash tags such as services/prs/v0.1.0 are nested directories.
func ValidateFeedReleaseRef(ref string) error {
	ref = normalizeReleaseRef(ref)
	if ref == "latest" {
		return nil
	}
	if strings.ContainsAny(ref, "\\:?#%*\"<>|") || strings.IndexFunc(ref, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return fmt.Errorf("release_ref %q contains characters a feed path cannot hold", ref)
	}
	for _, segment := range strings.Split(ref, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("release_ref %q must be a tag such as v1.2.3 or services/api/v1.2.3", ref)
		}
	}
	return nil
}

// escapeFeedPath escapes each segment of a "/"-separated feed path for a URL.
func escapeFeedPath(rel string) string {
	segments := strings.Split(rel, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
