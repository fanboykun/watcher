package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	watcherStateDir            = ".watcher"
	configSnapshotStoreDir     = "snapshots"
	configSnapshotManifestFile = "manifest.json"
	legacyConfigSnapshotDir    = ".watcher-snapshot"

	SnapshotSourceDeployment   = "deployment"
	SnapshotSourceActiveUpdate = "active_config_update"
	SnapshotSourceMigration    = "current_version_migration"
)

// ErrConfigSnapshotNotFound indicates that a retained release has no trusted snapshot.
var ErrConfigSnapshotNotFound = errors.New("config snapshot not found")

var configSnapshotMu sync.Mutex

type configSnapshotManifest struct {
	Version    string    `json:"version"`
	Source     string    `json:"source"`
	CapturedAt time.Time `json:"captured_at"`
}

// ConfigSnapshotPath returns the private snapshot directory for one version.
func ConfigSnapshotPath(installDir, version string) string {
	return filepath.Join(installDir, watcherStateDir, configSnapshotStoreDir, releaseStorageName(version))
}

// CaptureConfigSnapshot atomically stores all managed configuration for a version.
func CaptureConfigSnapshot(wcfg *WatcherConfig, version, source string) error {
	if wcfg == nil {
		return errors.New("watcher config is nil")
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("snapshot version is empty")
	}
	if strings.TrimSpace(source) == "" {
		return errors.New("snapshot source is empty")
	}

	configSnapshotMu.Lock()
	defer configSnapshotMu.Unlock()

	target := ConfigSnapshotPath(wcfg.InstallDir, version)
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fmt.Errorf("create snapshot store: %w", err)
	}
	temporary, err := os.MkdirTemp(parent, ".snapshot-")
	if err != nil {
		return fmt.Errorf("create temporary snapshot: %w", err)
	}
	defer os.RemoveAll(temporary)

	for i, svc := range wcfg.Services {
		serviceName := snapshotServiceName(svc, i)
		serviceRoot := filepath.Join(temporary, "services", serviceName)
		if strings.TrimSpace(svc.EnvFile) != "" {
			if err := writeManagedFile(filepath.Join(serviceRoot, "env"), svc.EnvFile, svc.EnvContent); err != nil {
				return fmt.Errorf("snapshot env %s for %s: %w", svc.EnvFile, serviceName, err)
			}
		}
		for _, file := range svc.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" {
				continue
			}
			category := "app"
			if normalizeConfigFileTarget(file.Target) == "release_dir" {
				category = "release"
			}
			if err := writeManagedFile(filepath.Join(serviceRoot, category), file.FilePath, file.Content); err != nil {
				return fmt.Errorf("snapshot %s config %s for %s: %w", category, file.FilePath, serviceName, err)
			}
		}
	}

	manifest := configSnapshotManifest{Version: version, Source: source, CapturedAt: time.Now().UTC()}
	manifestJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshot manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(temporary, configSnapshotManifestFile), manifestJSON, 0600); err != nil {
		return fmt.Errorf("write snapshot manifest: %w", err)
	}
	if err := replaceSnapshotDirectory(temporary, target); err != nil {
		return fmt.Errorf("publish snapshot for %s: %w", version, err)
	}
	return nil
}

// HasConfigSnapshot reports whether a trusted external snapshot exists for a version.
func HasConfigSnapshot(installDir, version string) bool {
	_, err := loadConfigSnapshotManifest(installDir, version)
	return err == nil
}

// DeleteConfigSnapshot removes the trusted snapshot owned by a deleted release.
func DeleteConfigSnapshot(installDir, version string) error {
	if err := os.RemoveAll(ConfigSnapshotPath(installDir, version)); err != nil {
		return fmt.Errorf("remove config snapshot for %s: %w", version, err)
	}
	return nil
}

// ValidateConfigSnapshot verifies snapshot provenance before any service is stopped.
func ValidateConfigSnapshot(installDir, version string) error {
	_, err := loadConfigSnapshotManifest(installDir, version)
	return err
}

// RestoreConfigSnapshot replaces managed files with the selected version's snapshot.
func RestoreConfigSnapshot(wcfg *WatcherConfig, version, currentDir string) error {
	if wcfg == nil {
		return errors.New("watcher config is nil")
	}
	if _, err := loadConfigSnapshotManifest(wcfg.InstallDir, version); err != nil {
		return err
	}

	snapshotRoot := ConfigSnapshotPath(wcfg.InstallDir, version)
	if err := removeCurrentManagedConfig(wcfg, currentDir); err != nil {
		return fmt.Errorf("clear current managed config: %w", err)
	}

	servicesDir := filepath.Join(snapshotRoot, "services")
	serviceEntries, err := os.ReadDir(servicesDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read snapshot services: %w", err)
	}
	for _, serviceEntry := range serviceEntries {
		if !serviceEntry.IsDir() {
			continue
		}
		serviceRoot := filepath.Join(servicesDir, serviceEntry.Name())
		for _, category := range []struct {
			name   string
			target string
		}{
			{name: "env", target: wcfg.InstallDir},
			{name: "app", target: wcfg.InstallDir},
			{name: "release", target: currentDir},
		} {
			if err := restoreSnapshotTree(filepath.Join(serviceRoot, category.name), category.target); err != nil {
				return fmt.Errorf("restore %s/%s: %w", serviceEntry.Name(), category.name, err)
			}
		}
	}
	return nil
}

// captureConfigSnapshot stores the deployer's managed configuration for a version.
func (d *Deployer) captureConfigSnapshot(version, source string) error {
	return CaptureConfigSnapshot(d.wcfg, version, source)
}

func loadConfigSnapshotManifest(installDir, version string) (*configSnapshotManifest, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil, fmt.Errorf("%w: version is empty", ErrConfigSnapshotNotFound)
	}
	manifestPath := filepath.Join(ConfigSnapshotPath(installDir, version), configSnapshotManifestFile)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w for version %s", ErrConfigSnapshotNotFound, version)
		}
		return nil, fmt.Errorf("read snapshot manifest for %s: %w", version, err)
	}
	var manifest configSnapshotManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode snapshot manifest for %s: %w", version, err)
	}
	if manifest.Version != version || strings.TrimSpace(manifest.Source) == "" || manifest.CapturedAt.IsZero() {
		return nil, fmt.Errorf("invalid snapshot manifest for version %s", version)
	}
	return &manifest, nil
}

func snapshotServiceName(svc ServiceConfig, index int) string {
	for _, candidate := range []string{svc.WindowsServiceName, svc.IISSiteName, svc.IISAppPool} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return safeSnapshotSegment(candidate)
		}
	}
	return fmt.Sprintf("service-%d", index+1)
}

func safeSnapshotSegment(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, "%%%X", r)
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "service"
	}
	return b.String()
}

func replaceSnapshotDirectory(source, target string) error {
	backup := fmt.Sprintf("%s.backup-%d", target, time.Now().UnixNano())
	hadTarget := false
	if _, err := os.Lstat(target); err == nil {
		hadTarget = true
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("preserve previous snapshot: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect previous snapshot: %w", err)
	}
	if err := os.Rename(source, target); err != nil {
		if hadTarget {
			_ = os.Rename(backup, target)
		}
		return fmt.Errorf("activate snapshot: %w", err)
	}
	if hadTarget {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous snapshot backup: %w", err)
		}
	}
	return nil
}

func removeCurrentManagedConfig(wcfg *WatcherConfig, currentDir string) error {
	for _, svc := range wcfg.Services {
		if strings.TrimSpace(svc.EnvFile) != "" {
			if err := removeManagedFile(wcfg.InstallDir, svc.EnvFile); err != nil {
				return err
			}
		}
		for _, file := range svc.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" {
				continue
			}
			root := wcfg.InstallDir
			if normalizeConfigFileTarget(file.Target) == "release_dir" {
				root = currentDir
			}
			if err := removeManagedFile(root, file.FilePath); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeManagedFile(root, relativePath string) error {
	target, err := managedFilePath(root, relativePath)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove managed file %s: %w", relativePath, err)
	}
	return nil
}

func restoreSnapshotTree(sourceDir, targetDir string) error {
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read snapshot file %s: %w", relativePath, err)
		}
		return writeManagedFile(targetDir, relativePath, string(content))
	})
}

// ReadServiceSnapshotEnv reads a historical environment through the same naming
// and path validation used by snapshot capture and rollback reconciliation.
func ReadServiceSnapshotEnv(wcfg *WatcherConfig, version string, serviceIndex int) (string, error) {
	if wcfg == nil || serviceIndex < 0 || serviceIndex >= len(wcfg.Services) {
		return "", errors.New("invalid snapshot service")
	}
	configSnapshotMu.Lock()
	defer configSnapshotMu.Unlock()
	if _, err := loadConfigSnapshotManifest(wcfg.InstallDir, version); err != nil {
		return "", err
	}
	svc := wcfg.Services[serviceIndex]
	root := filepath.Join(ConfigSnapshotPath(wcfg.InstallDir, version), "services", snapshotServiceName(svc, serviceIndex), "env")
	return readSnapshotManagedFile(root, svc.EnvFile)
}

// preserveConfigSnapshot retains historical configuration across failed
// redeploys, and removes incomplete snapshots for failed new releases.
func preserveConfigSnapshot(installDir, version string) (func(error) error, error) {
	configSnapshotMu.Lock()
	defer configSnapshotMu.Unlock()
	target := ConfigSnapshotPath(installDir, version)
	backup := ""
	if _, err := os.Stat(target); err == nil {
		backup, err = os.MkdirTemp(filepath.Dir(target), ".snapshot-backup-")
		if err != nil {
			return nil, err
		}
		if err := copyDir(target, backup); err != nil {
			os.RemoveAll(backup)
			return nil, fmt.Errorf("preserve config snapshot: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return func(cause error) error {
		configSnapshotMu.Lock()
		defer configSnapshotMu.Unlock()
		if cause != nil {
			if err := os.RemoveAll(target); err != nil {
				return errors.Join(cause, err)
			}
			if backup != "" {
				if err := os.Rename(backup, target); err != nil {
					return errors.Join(cause, fmt.Errorf("restore config snapshot: %w", err))
				}
			}
		} else if backup != "" {
			_ = os.RemoveAll(backup)
		}
		return cause
	}, nil
}
