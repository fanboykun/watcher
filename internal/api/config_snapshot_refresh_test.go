package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

func TestSyncServiceEnvRefreshesActiveVersionSnapshot(t *testing.T) {
	installDir := t.TempDir()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	watcher := database.Watcher{
		Name:             "snapshot-watcher",
		ServiceName:      "snapshot-service",
		MetadataURL:      "https://github.com/example/service",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       installDir,
		MaxKeptVersions:  3,
		CurrentVersion:   "v2",
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
		EnvContent:         "VALUE=old\n",
	}
	if err := db.Create(&service).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
	watcher.Services = []database.Service{service}
	if err := agent.CaptureConfigSnapshot(agent.WatcherConfigFromDB(&watcher), watcher.CurrentVersion, agent.SnapshotSourceDeployment); err != nil {
		t.Fatalf("capture initial snapshot: %v", err)
	}

	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan uint, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1))
	rec := authRequest(router, http.MethodPut, "/api/services/"+itoa(service.ID)+"/env", `{"env_content":"VALUE=new\n"}`, database.DefaultAuthPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	snapshotEnv := filepath.Join(agent.ConfigSnapshotPath(installDir, "v2"), "services", "snapshot-service", "env", ".env")
	content, err := os.ReadFile(snapshotEnv)
	if err != nil {
		t.Fatalf("read refreshed snapshot: %v", err)
	}
	if got, want := string(content), "VALUE=new\n"; got != want {
		t.Fatalf("snapshot env = %q, want %q", got, want)
	}

	rec = authRequest(router, http.MethodPut, "/api/services/"+itoa(service.ID)+"/env", `{"env_content":""}`, database.DefaultAuthPassword)
	if rec.Code != http.StatusOK {
		t.Fatalf("clear env status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	content, err = os.ReadFile(snapshotEnv)
	if err != nil {
		t.Fatalf("read cleared snapshot: %v", err)
	}
	if len(content) != 0 {
		t.Fatalf("cleared snapshot env = %q, want empty", content)
	}
}
