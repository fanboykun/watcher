package agent

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
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
}

// CheckForUpdate compares the current watcher version against the latest release
// from the watcher's own GitHub repository.
func CheckForUpdate(ctx context.Context, currentVersion, repoURL, token string) (*SelfUpdateInfo, error) {
	log := NewLogger("self-update")
	client := NewGitHubClient(token, log)

	release, err := client.FetchLatestRelease(ctx, repoURL)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}

	// Find the watcher zip asset
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".zip") && strings.Contains(asset.Name, "watcher") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	info := &SelfUpdateInfo{
		CurrentVersion:  currentVersion,
		LatestVersion:   release.TagName,
		UpdateAvailable: isNewer(release.TagName, currentVersion),
		DownloadURL:     downloadURL,
		PublishedAt:     release.PublishedAt,
	}

	return info, nil
}

// PerformSelfUpdate downloads the latest release, extracts the new watcher binary,
// and replaces the current executable. On Windows, it restarts via NSSM.
func PerformSelfUpdate(ctx context.Context, downloadURL, token, nssmPath, serviceName string) error {
	if downloadURL == "" {
		return fmt.Errorf("no download URL provided")
	}

	log := NewLogger("self-update")
	client := NewGitHubClient(token, log)

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
	if err := client.DownloadArtifact(ctx, downloadURL, tmpZip, 3); err != nil {
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

		// Restart the watcher service via NSSM
		if nssmPath != "" && serviceName != "" {
			log.Info("restarting watcher service", "service", serviceName)
			go func() {
				exec.Command(nssmPath, "restart", serviceName).CombinedOutput()
			}()
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
// Both should be in the format "vX.Y.Z" or "X.Y.Z".
func isNewer(latest, current string) bool {
	latest = strings.TrimPrefix(latest, "v")
	current = strings.TrimPrefix(current, "v")

	if current == "dev" || current == "" {
		return latest != ""
	}

	latestParts := strings.Split(latest, ".")
	currentParts := strings.Split(current, ".")

	for i := 0; i < 3; i++ {
		var l, c int
		if i < len(latestParts) {
			fmt.Sscanf(latestParts[i], "%d", &l)
		}
		if i < len(currentParts) {
			fmt.Sscanf(currentParts[i], "%d", &c)
		}
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}
