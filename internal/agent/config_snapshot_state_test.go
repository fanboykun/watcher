package agent

import (
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/database"
)

func TestReconcileConfigSnapshotRestoresPersistedServiceContent(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}

	watcher := database.Watcher{
		Name:             "snapshot-watcher",
		ServiceName:      "snapshot-service",
		MetadataURL:      "https://github.com/example/service",
		InstallDir:       t.TempDir(),
		CurrentVersion:   "v1",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	service := database.Service{
		WatcherID:          watcher.ID,
		ServiceType:        "nssm",
		WindowsServiceName: "snapshot-service",
		BinaryName:         "service.exe",
		EnvFile:            ".env",
		EnvContent:         "VERSION=old\n",
	}
	if err := db.Create(&service).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
	configFile := database.ServiceConfigFile{
		ServiceID: service.ID,
		FilePath:  "config/app.txt",
		Target:    "app_dir",
		Content:   "old-config",
	}
	if err := db.Create(&configFile).Error; err != nil {
		t.Fatalf("create config file: %v", err)
	}
	if err := db.Preload("Services").Preload("Services.ConfigFiles").First(&watcher, watcher.ID).Error; err != nil {
		t.Fatalf("reload watcher: %v", err)
	}
	if err := CaptureConfigSnapshot(WatcherConfigFromDB(&watcher), "v1", SnapshotSourceDeployment); err != nil {
		t.Fatalf("capture v1 config: %v", err)
	}

	if err := db.Model(&service).Update("env_content", "VERSION=latest\n").Error; err != nil {
		t.Fatalf("update latest environment: %v", err)
	}
	if err := db.Model(&configFile).Update("content", "latest-config").Error; err != nil {
		t.Fatalf("update latest config: %v", err)
	}

	services, err := ReconcileConfigSnapshot(db, watcher.ID, "v1")
	if err != nil {
		t.Fatalf("reconcile v1 config: %v", err)
	}
	if got, want := services[0].EnvContent, "VERSION=old\n"; got != want {
		t.Fatalf("in-memory environment = %q, want %q", got, want)
	}

	var restoredService database.Service
	if err := db.Preload("ConfigFiles").First(&restoredService, service.ID).Error; err != nil {
		t.Fatalf("load restored service: %v", err)
	}
	if got, want := restoredService.EnvContent, "VERSION=old\n"; got != want {
		t.Fatalf("persisted environment = %q, want %q", got, want)
	}
	if got, want := restoredService.ConfigFiles[0].Content, "old-config"; got != want {
		t.Fatalf("persisted config = %q, want %q", got, want)
	}
}
