package agent

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseGitHubURL extracts an owner and repository from supported GitHub URL forms.
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

// deriveServiceNameFromAsset derives a stable service name from a release asset filename.
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

// shouldIgnoreAssetForServiceDiscovery reports whether an asset is metadata rather than a deployable service.
func shouldIgnoreAssetForServiceDiscovery(assetName string) bool {
	name := strings.ToLower(strings.TrimSpace(assetName))
	return name == "version.json" || strings.HasSuffix(name, ".version.json")
}

// buildMetadataFromRelease builds service metadata by discovering deployable release assets.
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

// isKnownAssetSuffixToken reports whether a release asset token denotes a platform or architecture.
func isKnownAssetSuffixToken(token string) bool {
	_, ok := knownAssetSuffixTokens[strings.ToLower(strings.TrimSpace(token))]
	return ok
}

// isVersionLikeToken reports whether an asset-name token resembles a version.
func isVersionLikeToken(token string) bool {
	s := strings.TrimSpace(strings.ToLower(token))
	if s == "" {
		return false
	}
	s = strings.TrimPrefix(s, "v")
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
