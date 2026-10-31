package agent

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// SelfUpdateInfo contains version comparison info for self-update checks.
type SelfUpdateInfo struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	DownloadURL     string `json:"download_url"`
	PublishedAt     string `json:"published_at"`
	SHA256          string `json:"sha256,omitempty"` // verified before the binary is replaced
}

// SelfUpdateServiceName is the service key for Watcher's own releases in a feed manifest.
const SelfUpdateServiceName = "watcher"

// SelfUpdateSource locates Watcher's own releases: a GitHub repository, or an
// internal release feed for hosts that cannot reach GitHub.
type SelfUpdateSource struct {
	RepoURL  string
	Token    string   // GitHub only
	FeedAuth FeedAuth // http(s) feeds, including sigv4 for a private bucket
}

// feed returns the feed source when RepoURL is not a GitHub repository URL.
func (s SelfUpdateSource) feed(log *Logger) (feedSource, error) {
	if _, _, err := ParseGitHubURL(s.RepoURL); err == nil {
		return nil, nil
	}
	sourceType := SourceHTTP
	if isAbsFeedPath(strings.TrimSpace(s.RepoURL)) {
		sourceType = SourceFS
	}
	root, err := NormalizeFeedLocation(sourceType, s.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("WATCHER_REPO_URL is neither a GitHub repository nor a release feed: %w", err)
	}
	auth, err := NormalizeFeedAuth(s.FeedAuth)
	if err != nil {
		return nil, fmt.Errorf("WATCHER_REPO_AUTH_*: %w", err)
	}
	return newFeedSource(sourceType, root, auth, log), nil
}

// CheckForUpdate compares the current watcher version against the latest
// release from the watcher's own repository or release feed.
func CheckForUpdate(ctx context.Context, currentVersion string, src SelfUpdateSource) (*SelfUpdateInfo, error) {
	log := NewLogger("self-update")
	feed, err := src.feed(log)
	if err != nil {
		return nil, err
	}
	if feed != nil {
		meta, err := feed.FetchMetadata(ctx, "latest", SelfUpdateServiceName)
		if err != nil {
			return nil, fmt.Errorf("fetch release feed: %w", err)
		}
		svc, ok := meta.Services[SelfUpdateServiceName]
		if !ok {
			return nil, fmt.Errorf("release feed manifest has no %q service entry for Watcher's own releases", SelfUpdateServiceName)
		}
		return &SelfUpdateInfo{
			CurrentVersion:  currentVersion,
			LatestVersion:   svc.Version,
			UpdateAvailable: isNewer(svc.Version, currentVersion),
			DownloadURL:     svc.ArtifactURL,
			PublishedAt:     svc.PublishedAt,
			SHA256:          svc.SHA256,
		}, nil
	}

	client := NewGitHubClient(src.Token, log)
	release, err := client.FetchLatestRelease(ctx, src.RepoURL)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	info := &SelfUpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: isNewer(release.TagName, currentVersion),
		PublishedAt:     release.PublishedAt,
	}
	// Find the watcher zip asset
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".zip") && strings.Contains(asset.Name, "watcher") {
			info.DownloadURL = asset.BrowserDownloadURL
			if strings.HasPrefix(asset.Digest, "sha256:") {
				info.SHA256 = asset.Digest
			}
			break
		}
	}
	return info, nil
}

// downloadSelfUpdate fetches the release zip from the configured source and
// verifies its checksum when the release publishes one.
func downloadSelfUpdate(ctx context.Context, info *SelfUpdateInfo, src SelfUpdateSource, destPath string, log *Logger) error {
	expected, err := normalizeSHA256(info.SHA256)
	if err != nil {
		return err
	}
	feed, err := src.feed(log)
	if err != nil {
		return err
	}
	if feed != nil {
		err = feed.DownloadArtifact(ctx, info.DownloadURL, destPath, 3)
	} else {
		err = NewGitHubClient(src.Token, log).DownloadArtifact(ctx, info.DownloadURL, destPath, 3)
	}
	if err != nil {
		return err
	}
	if expected == "" {
		log.Warn("self-update release publishes no sha256; integrity not verified")
		return nil
	}
	actual, err := fileSHA256(destPath)
	if err != nil {
		return err
	}
	if actual != expected {
		os.Remove(destPath)
		return fmt.Errorf("update sha256 mismatch: release expects %s, downloaded file is %s", expected, actual)
	}
	return nil
}

// PerformSelfUpdate downloads the release described by info, extracts the new
// watcher binary, and replaces the current executable. On Windows, it restarts via NSSM.
func PerformSelfUpdate(ctx context.Context, info *SelfUpdateInfo, src SelfUpdateSource, nssmPath, serviceName string) error {
	if info == nil || info.DownloadURL == "" {
		return fmt.Errorf("no download URL provided")
	}

	log := NewLogger("self-update")

	// Get the path to the currently running executable
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	exeDir := filepath.Dir(exePath)
	exeName := filepath.Base(exePath)

	// Download the release zip to a temp location
	tmpZip := filepath.Join(exeDir, "update.zip")
	if err := downloadSelfUpdate(ctx, info, src, tmpZip, log); err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer os.Remove(tmpZip)

	// Extract just the watcher binary from the zip
	newExePath := filepath.Join(exeDir, exeName+".new")
	if err := extractFileFromZip(tmpZip, exeName, newExePath); err != nil {
		return fmt.Errorf("extract new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		// On Windows, we can't replace a running binary directly.
		// Rename current → .old, rename .new → current, then restart via NSSM.
		oldPath := exePath + ".old"
		os.Remove(oldPath) // remove any previous .old

		if err := os.Rename(exePath, oldPath); err != nil {
			os.Remove(newExePath)
			return fmt.Errorf("backup current binary: %w", err)
		}

		if err := os.Rename(newExePath, exePath); err != nil {
			// Try to restore the old binary
			os.Rename(oldPath, exePath)
			return fmt.Errorf("install new binary: %w", err)
		}

		// Restart the watcher service via NSSM.
		// This must be detached from the current process; otherwise the restart command
		// can be interrupted when NSSM stops this service process, leaving it stopped.
		if nssmPath != "" && serviceName != "" {
			log.Info("restarting watcher service", "service", serviceName)
			if err := ScheduleServiceRestart(nssmPath, serviceName, exeDir); err != nil {
				return fmt.Errorf("schedule service restart: %w", err)
			}
		}
	} else {
		// On Linux (dev), just swap the file
		if err := os.Rename(newExePath, exePath); err != nil {
			return fmt.Errorf("replace binary: %w", err)
		}
	}

	return nil
}

// GenerateUninstallScript generates a PowerShell uninstall script for the watcher.
func GenerateUninstallScript(nssmPath, serviceName, installDir string) string {
	return fmt.Sprintf(`# Watcher Uninstall Script
# Run as Administrator

$ErrorActionPreference = "Stop"

Write-Host "Stopping watcher service..."
& "%s" stop "%s" confirm 2>$null
Start-Sleep -Seconds 3

Write-Host "Removing watcher service..."
& "%s" remove "%s" confirm 2>$null

Write-Host "Removing install directory..."
Remove-Item -Path "%s" -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Watcher has been uninstalled successfully." -ForegroundColor Green
Write-Host "You may need to manually remove any watched service registrations."
`, nssmPath, serviceName, nssmPath, serviceName, installDir)
}

// extractFileFromZip extracts a single named file from a zip archive.
func extractFileFromZip(zipPath, fileName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		// Match by basename — the file might be nested
		if filepath.Base(f.Name) == fileName && !f.FileInfo().IsDir() {
			src, err := f.Open()
			if err != nil {
				return fmt.Errorf("open file in zip: %w", err)
			}
			defer src.Close()

			dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()|0755)
			if err != nil {
				return fmt.Errorf("create dest file: %w", err)
			}
			defer dst.Close()

			_, err = io.Copy(dst, src)
			return err
		}
	}

	return fmt.Errorf("file %q not found in zip", fileName)
}

// isNewer returns true if latestVersion is semantically newer than currentVersion.
// It accepts plain versions, prefixed tags, and artifact-like labels as long as a
// semantic version can be extracted from the string.
