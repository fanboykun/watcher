package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newWatcherFailureFixture(t *testing.T) (*RepoWatcher, *gorm.DB, database.Watcher) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.Service{}, &database.ServiceConfigFile{}, &database.DeployLog{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	watcher := database.Watcher{
		Name: "alpha", ServiceName: "api", InstallDir: t.TempDir(),
		CurrentVersion: "v1", Status: string(StatusHealthy),
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	log := newTestLogger()
	state := NewStateManager(db, watcher.ID, log, nil, nil)
	return &RepoWatcher{db: db, watcherID: watcher.ID, state: state, log: log}, db, watcher
}

func TestWatcherFailurePersistenceIntegration(t *testing.T) {
	cause := errors.New("start service api during deploy: target did not reach SERVICE_RUNNING")

	t.Run("successful version rollback creates linked successful attempt", func(t *testing.T) {
		watcherRuntime, db, watcher := newWatcherFailureFixture(t)
		watcherRuntime.wcfg = &WatcherConfig{InstallDir: watcher.InstallDir}
		if err := CaptureConfigSnapshot(watcherRuntime.wcfg, "v1", SnapshotSourceDeployment); err != nil {
			t.Fatalf("capture rollback config: %v", err)
		}
		if _, err := watcherRuntime.state.SetDeploying("v2", "v1"); err != nil {
			t.Fatal(err)
		}
		recovered := watcherRuntime.recordDeployFailure(&deployRecoveryError{cause: cause, rollbackVersion: "v1"}, "v2", "v1")
		if !recovered {
			t.Fatal("recordDeployFailure = false, want confirmed rollback")
		}

		var gotWatcher database.Watcher
		if err := db.First(&gotWatcher, watcher.ID).Error; err != nil {
			t.Fatal(err)
		}
		if gotWatcher.Status != string(StatusHealthy) || gotWatcher.CurrentVersion != "v1" {
			t.Fatalf("watcher status=%q version=%q, want healthy v1", gotWatcher.Status, gotWatcher.CurrentVersion)
		}
		var attempts []database.DeployLog
		if err := db.Where("watcher_id = ?", watcher.ID).Order("id").Find(&attempts).Error; err != nil {
			t.Fatal(err)
		}
		if len(attempts) != 2 || attempts[0].Kind != "deploy" || attempts[0].Status != "failed" || attempts[1].Kind != "rollback" || attempts[1].Status != "succeeded" {
			t.Fatalf("attempts = %#v, want failed deploy then successful rollback", attempts)
		}
		if attempts[1].ParentAttemptID == nil || *attempts[1].ParentAttemptID != attempts[0].ID {
			t.Fatalf("rollback parent=%v, want deploy ID %d", attempts[1].ParentAttemptID, attempts[0].ID)
		}
	})

	t.Run("same-version backup restoration remains failed without rollback attempt", func(t *testing.T) {
		watcherRuntime, db, watcher := newWatcherFailureFixture(t)
		if _, err := watcherRuntime.state.SetDeploying("v2", "v1"); err != nil {
			t.Fatal(err)
		}
		recovered := watcherRuntime.recordDeployFailure(&deployRecoveryError{cause: cause, restoredBackupVersion: "v2"}, "v2", "v1")
		if recovered {
			t.Fatal("backup restoration must not be reported as version rollback")
		}

		var gotWatcher database.Watcher
		_ = db.First(&gotWatcher, watcher.ID).Error
		if gotWatcher.Status != string(StatusFailed) || !strings.Contains(gotWatcher.LastError, "restored release backup for v2") {
			t.Fatalf("watcher status=%q error=%q, want failed with backup detail", gotWatcher.Status, gotWatcher.LastError)
		}
		var count int64
		db.Model(&database.DeployLog{}).Where("watcher_id = ? AND kind = ?", watcher.ID, "rollback").Count(&count)
		if count != 0 {
			t.Fatalf("rollback attempt count=%d, want 0", count)
		}
	})

	t.Run("rollback failure records both failures", func(t *testing.T) {
		watcherRuntime, db, watcher := newWatcherFailureFixture(t)
		if _, err := watcherRuntime.state.SetDeploying("v2", "v1"); err != nil {
			t.Fatal(err)
		}
		recoveryErr := &deployRecoveryError{cause: cause, rollbackErr: errors.New("previous service failed to start")}
		if watcherRuntime.recordDeployFailure(recoveryErr, "v2", "v1") {
			t.Fatal("failed rollback must not report recovery")
		}

		var attempts []database.DeployLog
		if err := db.Where("watcher_id = ?", watcher.ID).Order("id").Find(&attempts).Error; err != nil {
			t.Fatal(err)
		}
		if len(attempts) != 2 || attempts[0].Status != "failed" || attempts[1].Kind != "rollback" || attempts[1].Status != "failed" {
			t.Fatalf("attempts = %#v, want failed deploy and failed rollback", attempts)
		}
		var gotWatcher database.Watcher
		_ = db.First(&gotWatcher, watcher.ID).Error
		if gotWatcher.Status != string(StatusFailed) || !strings.Contains(gotWatcher.LastError, "rollback failed") {
			t.Fatalf("watcher status=%q error=%q, want rollback failure detail", gotWatcher.Status, gotWatcher.LastError)
		}
	})

	t.Run("first deployment failure creates no rollback attempt", func(t *testing.T) {
		watcherRuntime, db, watcher := newWatcherFailureFixture(t)
		db.Model(&database.Watcher{}).Where("id = ?", watcher.ID).Update("current_version", "")
		if _, err := watcherRuntime.state.SetDeploying("v1", ""); err != nil {
			t.Fatal(err)
		}
		if watcherRuntime.recordDeployFailure(&deployRecoveryError{cause: cause, noRollbackVersion: true}, "v1", "") {
			t.Fatal("first deploy failure must not report recovery")
		}
		var count int64
		db.Model(&database.DeployLog{}).Where("watcher_id = ? AND kind = ?", watcher.ID, "rollback").Count(&count)
		if count != 0 {
			t.Fatalf("rollback attempt count=%d, want 0", count)
		}
	})
}
