package api

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestServiceCollectionAPIsReturnEmptyArrays(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	watcher := database.Watcher{
		Name:             "empty-watcher",
		ServiceName:      "empty-service",
		MetadataURL:      "https://example.com/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       t.TempDir(),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}

	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(
		db,
		"nssm",
		cfg.LogDir,
		"test",
		"",
		".env",
		cfg,
		agent.NewLoggerWithWriter("api", io.Discard, "error"),
		nil,
		make(chan agent.CheckTrigger, 1),
		make(chan struct{}, 1),
		nil,
		make(chan struct{}, 1),
	)

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "flat services", path: "/api/services", want: "[]"},
		{name: "watcher services", path: "/api/watchers/" + itoa(watcher.ID) + "/services", want: "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := authRequest(router, http.MethodGet, tt.path, "", database.DefaultAuthPassword)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestWatcherAPIsEmbedEmptyServiceArrays(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	watcher := database.Watcher{
		Name:             "empty-watcher",
		ServiceName:      "empty-service",
		MetadataURL:      "https://example.com/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       t.TempDir(),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1))

	for _, path := range []string{"/api/watchers", "/api/watchers/" + itoa(watcher.ID)} {
		rec := authRequest(router, http.MethodGet, path, "", database.DefaultAuthPassword)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"services":null`) {
			t.Errorf("GET %s returned null services: %s", path, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"services":[]`) {
			t.Errorf("GET %s did not return an empty service array: %s", path, rec.Body.String())
		}
	}
}

func TestServiceAPIsEmbedEmptyConfigFileArrays(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	watcher := database.Watcher{
		Name:             "watcher",
		ServiceName:      "service",
		MetadataURL:      "https://example.com/version.json",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       t.TempDir(),
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	service := database.Service{
		WatcherID:          watcher.ID,
		ServiceType:        "nssm",
		WindowsServiceName: "service",
		BinaryName:         "service.exe",
	}
	if err := db.Create(&service).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1))

	paths := []string{
		"/api/services",
		"/api/services/" + itoa(service.ID),
		"/api/watchers/" + itoa(watcher.ID) + "/services",
	}
	for _, path := range paths {
		rec := authRequest(router, http.MethodGet, path, "", database.DefaultAuthPassword)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200; body=%s", path, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), `"config_files":null`) {
			t.Errorf("GET %s returned null config_files: %s", path, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"config_files":[]`) {
			t.Errorf("GET %s did not return an empty config_files array: %s", path, rec.Body.String())
		}
	}
}
