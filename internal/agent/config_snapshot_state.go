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
			for j := range service.ConfigFiles {
				file := &service.ConfigFiles[j]
				if err := tx.Model(file).Update("content", snapshotService.ConfigFiles[j].Content).Error; err != nil {
					return fmt.Errorf("update config file %d for service %d: %w", file.ID, service.ID, err)
				}
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

		for j := range services[i].ConfigFiles {
			file := &services[i].ConfigFiles[j]
			category := "app"
			if normalizeConfigFileTarget(file.Target) == "release_dir" {
				category = "release"
			}
			content, err := readSnapshotManagedFile(filepath.Join(serviceRoot, category), file.FilePath)
			if err != nil {
				return nil, fmt.Errorf("read snapshot %s config %s for %s: %w", category, file.FilePath, snapshotServiceName(services[i], i), err)
			}
			file.Content = content
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
