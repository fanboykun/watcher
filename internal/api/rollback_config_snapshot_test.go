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

func TestRollbackAPIRejectsVersionWithoutTrustedConfigSnapshot(t *testing.T) {
	installDir := t.TempDir()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "watcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{
		Name:             "rollback-watcher",
		ServiceName:      "rollback-service",
		MetadataURL:      "https://github.com/example/service",
		ReleaseRef:       "latest",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		InstallDir:       installDir,
		MaxKeptVersions:  3,
		CurrentVersion:   "v2",
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(installDir, "releases", "v1"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.AppConfig{APIPort: "8080", LogDir: t.TempDir()}
	router := NewRouter(db, "nssm", cfg.LogDir, "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), make(chan struct{}, 1), nil, make(chan struct{}, 1))
	rec := authRequest(router, http.MethodPost, "/api/watchers/"+itoa(watcher.ID)+"/rollback", `{"version":"v1"}`, database.DefaultAuthPassword)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}

	var attempts int64
	if err := db.Model(&database.DeployLog{}).Where("watcher_id = ?", watcher.ID).Count(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("rollback attempts = %d, want none before snapshot preflight", attempts)
	}
}
