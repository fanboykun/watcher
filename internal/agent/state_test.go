package agent

import (
	"testing"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStateManagerHasPendingManualDeploy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}); err != nil {
		t.Fatalf("migrate db: %v", err)
	}

	watcher := database.Watcher{
		Name:             "alpha",
		ServiceName:      "alpha-api",
		MetadataURL:      "https://github.com/example/repo",
		ReleaseRef:       "latest",
		InstallDir:       t.TempDir(),
		CheckIntervalSec: 60,
		DownloadRetries:  3,
		MaxKeptVersions:  3,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}

	state := NewStateManager(db, watcher.ID, NewLogger("test"), nil)
	if state.HasPendingManualDeploy() {
		t.Fatalf("expected no pending manual deploy initially")
	}

	log := database.DeployLog{
		WatcherID:   watcher.ID,
		TriggeredBy: "manual",
		Version:     "alpha-api/v0.1.0",
		Status:      string(StatusDeploying),
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatalf("create manual deploy log: %v", err)
	}

	if !state.HasPendingManualDeploy() {
		t.Fatalf("expected pending manual deploy to be detected")
	}
}
