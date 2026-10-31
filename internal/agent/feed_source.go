package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

// An internal release feed is a plain directory tree served over HTTP(S):
//
//	<feed root>/version.json                    latest pointer, published last
//	<feed root>/releases.json                   release index (pinning/catalog)
//	<feed root>/releases/<version>/version.json per-release manifest, write-once
//	<feed root>/releases/<version>/<artifact>   artifact, write-once
//
// See docs/adr/0002-internal-release-feed.md.
const feedManifestName = "version.json"

const (
	feedMetadataTimeout  = 30 * time.Second
	maxFeedManifestBytes = 2 << 20
)

// feedTransport is shared so feed polls reuse connections. It carries no
// credentials of its own: the GitHub token is never sent to feed hosts, and a
// feed credential is added per source by feedAuthTransport.
var feedTransport = func() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = feedMetadataTimeout
	return t
}()

// httpFeedSource reads releases from an internal HTTP(S) feed.
type httpFeedSource struct {
	root   string
	client *http.Client
	log    *Logger
	cache  *feedPollCache // nil outside a watcher's poll loop
}

func newHTTPFeedSource(root string, auth FeedAuth, log *Logger) *httpFeedSource {
	// No client timeout: artifact downloads are bounded by ctx, metadata by feedMetadataTimeout.
	transport := NewTraceTransport(newFeedAuthTransport(feedTransport, root, auth), log)
	return &httpFeedSource{root: root, log: log, client: &http.Client{Transport: transport}}
}

// NormalizeFeedRoot validates a feed root URL. A pasted manifest URL
// (".../version.json") is accepted and trimmed to its feed root.
func NormalizeFeedRoot(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("feed URL must be an absolute http:// or https:// URL")
	}
	if u.User != nil {
		return "", errors.New("feed URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("feed URL must not contain a query string or fragment")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/"+feedManifestName)
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = ""
	return u.String(), nil
}

// FetchMetadata reads the latest manifest, or releases/<ref>/version.json for a
// pinned ref, and resolves artifact URLs against it. Within a poll loop it
// revalidates with ETag/Last-Modified, and a 304 reuses the cached manifest.
func (s *httpFeedSource) FetchMetadata(ctx context.Context, releaseRef, _ string) (*VersionMetadata, error) {
	rel, err := feedManifestPath(releaseRef)
	if err != nil {
		return nil, err
	}
	docURL, err := s.feedURL(rel)
	if err != nil {
		return nil, err
	}
	prev := s.cache.lookup(ctx, docURL.String())
	var meta VersionMetadata
	fetched, err := s.fetchJSON(ctx, docURL, &meta, prev)
	if err != nil {
		return nil, err
	}
	if fetched.notModified {
		return prev.unchanged(), nil
	}
	if err := resolveArtifactURLs(meta.Services, docURL); err != nil {
		return nil, err
	}
	// Servers that send no validators are simply fetched in full every poll.
	if fetched.etag != "" || fetched.lastModified != "" {
		s.cache.store(feedPollEntry{key: docURL.String(), etag: fetched.etag, lastModified: fetched.lastModified, meta: cloneMetadata(&meta)})
	}
	return &meta, nil
}

func (s *httpFeedSource) fetchIndex(ctx context.Context) ([]feedRelease, error) {
	docURL, err := s.feedURL(feedIndexName)
	if err != nil {
		return nil, err
	}
	var index feedIndex
	if _, err := s.fetchJSON(ctx, docURL, &index, nil); err != nil {
		return nil, err
	}
	for _, release := range index.Releases {
		if err := resolveArtifactURLs(release.Services, docURL); err != nil {
			return nil, fmt.Errorf("release %q: %w", release.Tag, err)
		}
	}
	return index.Releases, nil
}

func (s *httpFeedSource) setPollCache(cache *feedPollCache) { s.cache = cache }

// feedURL locates rel, a "/"-separated path below the feed root.
func (s *httpFeedSource) feedURL(rel string) (*url.URL, error) {
	root, err := NormalizeFeedRoot(s.root)
	if err != nil {
		return nil, err
	}
	docURL, err := url.Parse(root + "/" + escapeFeedPath(rel))
	if err != nil {
		return nil, fmt.Errorf("build feed URL: %w", err)
	}
	return docURL, nil
}

type feedFetch struct {
	etag         string
	lastModified string
	notModified  bool
}

// fetchJSON decodes the feed document at docURL. With prev it sends
// conditional headers and reports a 304 without decoding anything.
func (s *httpFeedSource) fetchJSON(ctx context.Context, docURL *url.URL, v any, prev *feedPollEntry) (feedFetch, error) {
	name := path.Base(docURL.Path)
	ctx, cancel := context.WithTimeout(ctx, feedMetadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, docURL.String(), nil)
	if err != nil {
		return feedFetch{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if prev != nil {
		if prev.etag != "" {
			req.Header.Set("If-None-Match", prev.etag)
		}
		if prev.lastModified != "" {
			req.Header.Set("If-Modified-Since", prev.lastModified)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return feedFetch{}, fmt.Errorf("fetch %s: %w", name, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotModified && prev != nil:
		return feedFetch{notModified: true}, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return feedFetch{}, fmt.Errorf("feed authentication failed (HTTP %d) for %s; check the feed credentials", resp.StatusCode, docURL)
	case resp.StatusCode == http.StatusNotFound:
		return feedFetch{}, fmt.Errorf("HTTP 404 -- no %s at %s; check the feed URL and that a release has been published", name, docURL)
	case resp.StatusCode != http.StatusOK:
		return feedFetch{}, fmt.Errorf("feed returned HTTP %d for %s", resp.StatusCode, docURL)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedManifestBytes+1))
	if err != nil {
		return feedFetch{}, fmt.Errorf("read %s: %w", name, err)
	}
	if len(data) > maxFeedManifestBytes {
		return feedFetch{}, fmt.Errorf("%s exceeds %d bytes", name, maxFeedManifestBytes)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return feedFetch{}, fmt.Errorf("decode %s: %w", name, err)
	}
	return feedFetch{etag: resp.Header.Get("ETag"), lastModified: resp.Header.Get("Last-Modified")}, nil
}

// resolveArtifactURLs makes each artifact_url absolute relative to the manifest
// that lists it, so one feed can be served from any host or path.
func resolveArtifactURLs(services map[string]ServiceMeta, manifestURL *url.URL) error {
	for name, svc := range services {
		if strings.TrimSpace(svc.ArtifactURL) == "" {
			continue
		}
		ref, err := url.Parse(strings.TrimSpace(svc.ArtifactURL))
		if err != nil {
			return fmt.Errorf("service %q: invalid artifact_url: %w", name, err)
		}
		resolved := manifestURL.ResolveReference(ref)
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return fmt.Errorf("service %q: artifact_url must resolve to an http(s) URL", name)
		}
		svc.ArtifactURL = resolved.String()
		services[name] = svc
	}
	return nil
}

// DownloadArtifact streams the artifact to destPath, retrying with backoff.
func (s *httpFeedSource) DownloadArtifact(ctx context.Context, artifactURL, destPath string, retries int) error {
	retries = max(retries, 1)
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		s.log.Info("downloading artifact", "url", artifactURL, "attempt", attempt)
		lastErr = s.download(ctx, artifactURL, destPath)
		if lastErr == nil {
			s.log.Info("artifact downloaded", "dest", destPath)
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.log.Warn("download attempt failed", "attempt", attempt, "error", lastErr)
		if attempt < retries {
			if err := sleepContext(ctx, time.Duration(attempt*attempt)*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("download failed after %d attempts: %w", retries, lastErr)
}

func (s *httpFeedSource) download(ctx context.Context, artifactURL, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("download returned HTTP %d: feed authentication failed; check the feed credentials", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}
	return writeFileAtomic(destPath, resp.Body)
}

// writeFileAtomic streams r to a temp file and renames it over destPath, so a
// partial download is never mistaken for a complete artifact.
func writeFileAtomic(destPath string, r io.Reader) error {
	tmp := destPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	_, copyErr := io.Copy(f, r)
	closeErr := f.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
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
