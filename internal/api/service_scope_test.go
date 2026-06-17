package api

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestDeleteServiceRequiresServiceToBelongToWatcher(t *testing.T) {
	t.Parallel()

	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new db: %v", err)
	}

	w1 := database.Watcher{
		Name:             "watcher-one",
		ServiceName:      "svc-one",
		MetadataURL:      "https://example.com/one/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       filepath.Join(t.TempDir(), "one"),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&w1).Error; err != nil {
		t.Fatalf("create watcher one: %v", err)
	}

	w2 := database.Watcher{
		Name:             "watcher-two",
		ServiceName:      "svc-two",
		MetadataURL:      "https://example.com/two/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       filepath.Join(t.TempDir(), "two"),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&w2).Error; err != nil {
		t.Fatalf("create watcher two: %v", err)
	}

	svc := database.Service{
		WatcherID:          w2.ID,
		ServiceType:        "nssm",
		WindowsServiceName: "svc-two",
		BinaryName:         "app.exe",
	}
	if err := db.Create(&svc).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}

	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, nil, make(chan uint, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1))

	rec := authRequest(router, http.MethodDelete, "/api/watchers/"+itoa(w1.ID)+"/services/"+itoa(svc.ID), "", "watcher")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete mismatched service status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}

	var stillThere database.Service
	if err := db.First(&stillThere, svc.ID).Error; err != nil {
		t.Fatalf("expected service to remain after mismatched delete, got error: %v", err)
	}
}

func itoa(id uint) string {
	return fmt.Sprintf("%d", id)
}
