package agent

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// releaseStorageName encodes a version into a filesystem-safe release directory name.
func releaseStorageName(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	return url.PathEscape(version)
}

// restoreReleaseVersion decodes a release directory name back into its version.
func restoreReleaseVersion(storage string) string {
	storage = strings.TrimSpace(storage)
	if storage == "" {
		return storage
	}
	restored, err := url.PathUnescape(storage)
	if err != nil {
		return storage
	}
	return restored
}

// currentVersionFromCurrentDir resolves the active version from the current release link.
func currentVersionFromCurrentDir(installDir string) (string, error) {
	currentDir := filepath.Join(installDir, "current")
	releasesDir := filepath.Join(installDir, "releases")

	target, err := os.Readlink(currentDir)
	if err != nil || strings.TrimSpace(target) == "" {
		target, err = filepath.EvalSymlinks(currentDir)
		if err != nil {
			return "", nil
		}
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(currentDir), target)
	}
	target = filepath.Clean(target)

	rel, err := filepath.Rel(releasesDir, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", nil
	}

	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", nil
	}

	return restoreReleaseVersion(parts[0]), nil
}

type releasePromotion struct {
	releaseDir string
	backupDir  string
}

// restore restores the operation.
func (p *releasePromotion) restore() error {
	if p == nil {
		return nil
	}
	if err := removePath(p.releaseDir); err != nil {
		return fmt.Errorf("remove promoted release: %w", err)
	}
	if p.backupDir == "" {
		return nil
	}
	if err := os.Rename(p.backupDir, p.releaseDir); err != nil {
		return fmt.Errorf("restore release backup: %w", err)
	}
	return nil
}

// cleanup removes the operation.
func (p *releasePromotion) cleanup() error {
	if p == nil || p.backupDir == "" {
		return nil
	}
	return removePath(p.backupDir)
}

// prepareRelease prepares release.
func (d *Deployer) prepareRelease(stagedDir string) error {
	if err := d.validateReleaseArtifacts(stagedDir); err != nil {
		return err
	}
	if err := d.writeReleaseConfigFiles(stagedDir); err != nil {
		return err
	}
	return nil
}

// validateReleaseArtifacts validates release artifacts.
func (d *Deployer) validateReleaseArtifacts(releaseDir string) error {
	for _, svc := range d.wcfg.Services {
		if svc.ServiceType == "iis" || svc.ServiceType == "static" {
			continue
		}

		binaryName := strings.TrimSpace(svc.BinaryName)
		if binaryName == "" {
			return fmt.Errorf("binary_name is empty for service %s", svc.WindowsServiceName)
		}
		binaryPath := filepath.Join(releaseDir, binaryName)
		rel, err := filepath.Rel(releaseDir, binaryPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("binary %q for service %s escapes the release directory", binaryName, svc.WindowsServiceName)
		}
		info, err := os.Stat(binaryPath)
		if err != nil {
			return fmt.Errorf("inspect binary %q for service %s: %w", binaryName, svc.WindowsServiceName, err)
		}
		if info.IsDir() {
			return fmt.Errorf("binary %q for service %s is a directory", binaryName, svc.WindowsServiceName)
		}
	}
	return nil
}

// promoteRelease promotes release.
func (d *Deployer) promoteRelease(stagedDir, releaseDir string) (*releasePromotion, error) {
	promotion := &releasePromotion{releaseDir: releaseDir}
	if _, err := os.Lstat(releaseDir); err == nil {
		promotion.backupDir = filepath.Join(
			d.wcfg.InstallDir,
			fmt.Sprintf(".watcher-release-backup-%d", time.Now().UnixNano()),
		)
		if err := os.Rename(releaseDir, promotion.backupDir); err != nil {
			return nil, fmt.Errorf("backup existing release %s: %w", releaseDir, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("inspect existing release %s: %w", releaseDir, err)
	}

	if err := os.Rename(stagedDir, releaseDir); err == nil {
		return promotion, nil
	} else {
		d.lWarn("release rename failed, falling back to copy", "error", err)
		if cleanupErr := removePath(releaseDir); cleanupErr != nil {
			restoreErr := promotion.restore()
			failure := errors.Join(fmt.Errorf("prepare release copy target: %w", cleanupErr), fmt.Errorf("rename staged release: %w", err))
			if restoreErr != nil {
				failure = errors.Join(failure, restoreErr)
			}
			return nil, failure
		}
		if copyErr := copyDir(stagedDir, releaseDir); copyErr != nil {
			restoreErr := promotion.restore()
			failure := errors.Join(fmt.Errorf("rename staged release: %w", err), fmt.Errorf("copy staged release: %w", copyErr))
			if restoreErr != nil {
				failure = errors.Join(failure, restoreErr)
			}
			return nil, failure
		}
	}
	return promotion, nil
}

// cleanupPromotion removes promotion.
func (d *Deployer) cleanupPromotion(promotion *releasePromotion) {
	if err := promotion.cleanup(); err != nil {
		d.lWarn("failed to remove release backup", "path", promotion.backupDir, "error", err)
	}
}
