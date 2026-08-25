package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ── Version retention ─────────────────────────────────────────────────

// ReleaseInfo describes a version directory on disk.
type ReleaseInfo struct {
	Version     string    `json:"version"`
	Path        string    `json:"path"`
	SizeBytes   int64     `json:"size_bytes"`
	ModTime     time.Time `json:"mod_time"`
	IsCurrent   bool      `json:"is_current"`
	HasSnapshot bool      `json:"has_snapshot"`
}

// ListAvailableVersions returns the release directories on disk for a given installDir.
func ListAvailableVersions(installDir string) ([]ReleaseInfo, error) {
	releasesDir := filepath.Join(installDir, "releases")
	entries, err := os.ReadDir(releasesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read releases dir: %w", err)
	}

	// Determine current version by reading the junction/symlink target
	currentDir := filepath.Join(installDir, "current")
	currentTarget, _ := os.Readlink(currentDir)
	// On Windows with junctions, Readlink may fail — try filepath.EvalSymlinks
	if currentTarget == "" {
		currentTarget, _ = filepath.EvalSymlinks(currentDir)
	}

	var versions []ReleaseInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fullPath := filepath.Join(releasesDir, e.Name())
		ri := ReleaseInfo{
			Version:     restoreReleaseVersion(e.Name()),
			Path:        fullPath,
			ModTime:     info.ModTime(),
			SizeBytes:   dirSize(fullPath),
			IsCurrent:   fullPath == currentTarget,
			HasSnapshot: HasConfigSnapshot(installDir, restoreReleaseVersion(e.Name())),
		}
		versions = append(versions, ri)
	}

	// Sort by mod time descending (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ModTime.After(versions[j].ModTime)
	})

	return versions, nil
}

// CleanOldReleases removes old release directories, keeping only the `keep` most recent.
func CleanOldReleases(installDir string, keep int) error {
	versions, err := ListAvailableVersions(installDir)
	if err != nil {
		return err
	}

	if len(versions) <= keep {
		return nil
	}

	for _, v := range versions[keep:] {
		if v.IsCurrent {
			continue // never delete the current version
		}
		if err := os.RemoveAll(v.Path); err != nil {
			return fmt.Errorf("remove old release %s: %w", v.Version, err)
		}
		if err := DeleteConfigSnapshot(installDir, v.Version); err != nil {
			return err
		}
		if err := DeleteReleaseDeploymentMetadata(installDir, v.Version); err != nil {
			return err
		}
	}
	return nil
}

// dirSize calculates the total size of all files in a directory tree.
func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		size += info.Size()
		return nil
	})
	return size
}

// DeleteVersion removes a specific version directory.
func DeleteVersion(installDir, version string) error {
	versions, err := ListAvailableVersions(installDir)
	if err != nil {
		return err
	}

	for _, v := range versions {
		if v.Version == version {
			if v.IsCurrent {
				return fmt.Errorf("cannot delete the current active version")
			}
			if err := os.RemoveAll(v.Path); err != nil {
				return fmt.Errorf("failed to remove version %s: %w", version, err)
			}
			if err := DeleteConfigSnapshot(installDir, version); err != nil {
				return err
			}
			if err := DeleteReleaseDeploymentMetadata(installDir, version); err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("version %s not found on disk", version)
}
