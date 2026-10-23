package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

// ReconcileConfigSnapshot updates persisted and in-memory service configuration
// to match the trusted snapshot for version after a successful rollback.
func ReconcileConfigSnapshot(db *gorm.DB, watcherID uint, version string) ([]ServiceConfig, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}

	var watcher database.Watcher
	if err := db.
		Preload("Services", func(tx *gorm.DB) *gorm.DB { return tx.Order("id ASC") }).
		Preload("Services.ConfigFiles", func(tx *gorm.DB) *gorm.DB { return tx.Order("id ASC") }).
		First(&watcher, watcherID).Error; err != nil {
		return nil, fmt.Errorf("load watcher config: %w", err)
	}

	wcfg := WatcherConfigFromDB(&watcher)
	services, err := readConfigSnapshotServices(wcfg, version)
	if err != nil {
		return nil, err
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		for i := range watcher.Services {
			service := &watcher.Services[i]
			snapshotService := services[i]
			if strings.TrimSpace(service.EnvFile) != "" {
				if err := tx.Model(service).Update("env_content", snapshotService.EnvContent).Error; err != nil {
					return fmt.Errorf("update environment for service %d: %w", service.ID, err)
				}
			}
			files := make([]database.RevisionConfigFile, 0, len(snapshotService.ConfigFiles))
			for _, file := range snapshotService.ConfigFiles {
				files = append(files, database.RevisionConfigFile{FilePath: file.FilePath, Target: file.Target, Content: file.Content})
			}
			if err := replaceServiceManagedFiles(tx, service.ID, files); err != nil {
				return fmt.Errorf("restore managed config for service %d: %w", service.ID, err)
			}
		}
		if err := tx.Model(&database.Watcher{}).Where("id = ?", watcherID).
			UpdateColumn("updated_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("mark watcher config updated: %w", err)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("persist restored config snapshot for %s: %w", version, err)
	}

	return services, nil
}

// readConfigSnapshotServices returns copies of the configured services with
// managed contents loaded from one trusted version snapshot.
func readConfigSnapshotServices(wcfg *WatcherConfig, version string) ([]ServiceConfig, error) {
	if wcfg == nil {
		return nil, errors.New("watcher config is nil")
	}

	configSnapshotMu.Lock()
	defer configSnapshotMu.Unlock()

	if _, err := loadConfigSnapshotManifest(wcfg.InstallDir, version); err != nil {
		return nil, err
	}

	services := make([]ServiceConfig, len(wcfg.Services))
	copy(services, wcfg.Services)
	for i := range services {
		services[i].ConfigFiles = append([]ConfigFile(nil), wcfg.Services[i].ConfigFiles...)
		serviceRoot := filepath.Join(ConfigSnapshotPath(wcfg.InstallDir, version), "services", snapshotServiceName(services[i], i))

		if strings.TrimSpace(services[i].EnvFile) != "" {
			content, err := readSnapshotManagedFile(filepath.Join(serviceRoot, "env"), services[i].EnvFile)
			if err != nil {
				return nil, fmt.Errorf("read snapshot env %s for %s: %w", services[i].EnvFile, snapshotServiceName(services[i], i), err)
			}
			services[i].EnvContent = content
		}

		services[i].ConfigFiles = nil
		for _, category := range []struct{ name, target string }{{"app", "app_dir"}, {"release", "release_dir"}} {
			files, err := readSnapshotConfigFiles(filepath.Join(serviceRoot, category.name), category.target)
			if err != nil {
				return nil, fmt.Errorf("read snapshot config for %s: %w", snapshotServiceName(services[i], i), err)
			}
			services[i].ConfigFiles = append(services[i].ConfigFiles, files...)
		}
	}
	return services, nil
}

// readSnapshotManagedFile reads a configured relative path without allowing it
// to escape its snapshot category directory.
func readSnapshotManagedFile(root, relativePath string) (string, error) {
	path, err := managedFilePath(root, relativePath)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// The private snapshot trees are also the file inventory, including historical
// snapshots created before candidates could add or remove managed files.
func readSnapshotConfigFiles(root, target string) ([]ConfigFile, error) {
	var files []ConfigFile
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains symlink %s", filePath)
		}
		relative, err := filepath.Rel(root, filePath)
		if err != nil {
			return err
		}
		content, err := readSnapshotManagedFile(root, relative)
		if err != nil {
			return err
		}
		files = append(files, ConfigFile{FilePath: filepath.ToSlash(relative), Target: target, Content: content})
		return nil
	})
	return files, err
}
