package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fsFeedSource reads a release feed from a local directory or SMB/UNC share,
// for networks where releases land on a file server rather than a web server.
// The layout is the same as an HTTP feed.
type fsFeedSource struct {
	root  string
	log   *Logger
	cache *feedPollCache // nil outside a watcher's poll loop
}

func newFSFeedSource(root string, log *Logger) *fsFeedSource {
	return &fsFeedSource{root: root, log: log}
}

// NormalizeFSFeedRoot validates a feed directory. It must be absolute: a UNC
// share (\\server\share\app), a drive path (D:\releases\app) or, off Windows,
// a rooted path. A pasted path to version.json is trimmed to its directory.
func NormalizeFSFeedRoot(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if strings.Contains(p, "://") {
		return "", errors.New("feed path must be a local or UNC path, not a URL")
	}
	if !isAbsFeedPath(p) {
		return "", errors.New(`feed path must be absolute, for example \\fileserver\releases\app or D:\releases\app`)
	}
	p = strings.TrimRight(p, `\/`)
	p = strings.TrimSuffix(strings.TrimSuffix(p, `\`+feedManifestName), "/"+feedManifestName)
	p = strings.TrimRight(p, `\/`)
	if !isAbsFeedPath(p) || (len(p) == 2 && p[1] == ':') {
		return "", errors.New("feed path must name a directory below the drive or share root")
	}
	return p, nil
}

// isAbsFeedPath recognizes Windows drive and UNC paths on every OS, so paths
// configured for a Windows host validate the same way in tests and in the API.
func isAbsFeedPath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) {
		return true
	}
	if strings.HasPrefix(p, `\\`) {
		parts := strings.FieldsFunc(p[2:], func(r rune) bool { return r == '\\' || r == '/' })
		return len(parts) >= 2
	}
	return false
}

// FetchMetadata reads the latest manifest, or releases/<ref>/version.json for a
// pinned ref, and resolves artifact paths against it. Within a poll loop an
// unchanged file (same modification time and size) reuses the cached manifest.
func (s *fsFeedSource) FetchMetadata(ctx context.Context, releaseRef, _ string) (*VersionMetadata, error) {
	rel, err := feedManifestPath(releaseRef)
	if err != nil {
		return nil, err
	}
	root, err := NormalizeFSFeedRoot(s.root)
	if err != nil {
		return nil, err
	}
	manifestPath := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(manifestPath)
	if err != nil {
		return nil, describeFeedFileError(manifestPath, err)
	}
	if prev := s.cache.lookup(ctx, manifestPath); prev != nil && prev.modTime.Equal(info.ModTime()) && prev.size == info.Size() {
		return prev.unchanged(), nil
	}
	var meta VersionMetadata
	dir, err := s.readJSON(rel, &meta)
	if err != nil {
		return nil, err
	}
	if err := resolveArtifactPaths(meta.Services, dir); err != nil {
		return nil, err
	}
	s.cache.store(feedPollEntry{key: manifestPath, modTime: info.ModTime(), size: info.Size(), meta: cloneMetadata(&meta)})
	return &meta, nil
}

func (s *fsFeedSource) setPollCache(cache *feedPollCache) { s.cache = cache }

func (s *fsFeedSource) fetchIndex(context.Context) ([]feedRelease, error) {
	var index feedIndex
	dir, err := s.readJSON(feedIndexName, &index)
	if err != nil {
		return nil, err
	}
	for _, release := range index.Releases {
		if err := resolveArtifactPaths(release.Services, dir); err != nil {
			return nil, fmt.Errorf("release %q: %w", release.Tag, err)
		}
	}
	return index.Releases, nil
}

// readJSON decodes the feed file at rel, a "/"-separated path below the feed
// root, and returns its directory: relative paths inside it resolve against it.
func (s *fsFeedSource) readJSON(rel string, v any) (string, error) {
	root, err := NormalizeFSFeedRoot(s.root)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	data, err := readFeedFile(path)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return "", fmt.Errorf("decode %s: %w", rel, err)
	}
	return filepath.Dir(path), nil
}

func readFeedFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, describeFeedFileError(path, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFeedManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxFeedManifestBytes {
		return nil, fmt.Errorf("feed manifest exceeds %d bytes", maxFeedManifestBytes)
	}
	return data, nil
}

// describeFeedFileError explains the two failures operators hit on shares.
func describeFeedFileError(path string, err error) error {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s not found; check the feed path and that a release has been published", path)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf(`access denied reading %s; grant read access to the account Watcher runs as (LocalSystem reaches shares as the computer account DOMAIN\HOST$)`, path)
	}
	return fmt.Errorf("open %s: %w", path, err)
}

// resolveArtifactPaths makes each artifact_url an absolute path. Relative
// paths use "/" like an HTTP feed and resolve against the manifest directory.
func resolveArtifactPaths(services map[string]ServiceMeta, manifestDir string) error {
	for name, svc := range services {
		raw := strings.TrimSpace(svc.ArtifactURL)
		if raw == "" {
			continue
		}
		if strings.Contains(raw, "://") {
			return fmt.Errorf("service %q: file share feeds reference artifacts by path, not URL (%q)", name, raw)
		}
		if !isAbsFeedPath(raw) {
			raw = filepath.Join(manifestDir, filepath.FromSlash(raw))
		}
		svc.ArtifactURL = raw
		services[name] = svc
	}
	return nil
}

// DownloadArtifact copies the artifact from the share, retrying with backoff
// because SMB reads fail transiently during file-server failover.
func (s *fsFeedSource) DownloadArtifact(ctx context.Context, artifactPath, destPath string, retries int) error {
	retries = max(retries, 1)
	var lastErr error
	for attempt := 1; attempt <= retries; attempt++ {
		s.log.Info("copying artifact", "path", artifactPath, "attempt", attempt)
		lastErr = copyFeedFile(ctx, artifactPath, destPath)
		if lastErr == nil {
			s.log.Info("artifact copied", "dest", destPath)
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		s.log.Warn("copy attempt failed", "attempt", attempt, "error", lastErr)
		if attempt < retries {
			if err := sleepContext(ctx, time.Duration(attempt*attempt)*time.Second); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("copy failed after %d attempts: %w", retries, lastErr)
}

func copyFeedFile(ctx context.Context, src, destPath string) error {
	f, err := os.Open(src)
	if err != nil {
		return describeFeedFileError(src, err)
	}
	defer f.Close()
	return writeFileAtomic(destPath, contextReader{ctx: ctx, r: f})
}

// contextReader stops a long copy once ctx is canceled.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
