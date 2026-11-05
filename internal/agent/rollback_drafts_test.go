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

// newRollbackDraftFixture stores v1's snapshot with env "OLD" for one service.
func newRollbackDraftFixture(t *testing.T) (*gorm.DB, database.Watcher, database.Service) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.Service{}, &database.ServiceConfigFile{}, &database.ServiceConfigRevision{}); err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "alpha", ServiceName: "api", MetadataURL: "https://github.com/o/r", InstallDir: t.TempDir(), CurrentVersion: "v2"}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	service := database.Service{WatcherID: watcher.ID, ServiceType: "nssm", WindowsServiceName: "api-svc", BinaryName: "api.exe", EnvFile: ".env", EnvContent: "ACTIVE=1"}
	if err := db.Create(&service).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := WatcherConfigFromDB(&watcher)
	snapshot.Services = []ServiceConfig{{ID: service.ID, WindowsServiceName: "api-svc", EnvFile: ".env", EnvContent: "OLD=1"}}
	if err := CaptureConfigSnapshot(snapshot, "v1", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	return db, watcher, service
}

func snapshotEnv(t *testing.T, db *gorm.DB, watcherID uint, version string) string {
	t.Helper()
	config, err := ReadRollbackConfig(db, watcherID, version)
	if err != nil {
		t.Fatal(err)
	}
	return config.Services[0].SnapshotEnv
}

func TestStageRollbackDraftsAppliesThenRestoresOnFailure(t *testing.T) {
	db, watcher, service := newRollbackDraftFixture(t)
	files := []database.RevisionConfigFile{{FilePath: "config.json", Target: "app_dir", Content: "{}"}}
	if err := db.Create(&database.ServiceConfigRevision{ServiceID: service.ID, TargetVersion: "v1", EnvContent: "NEW=1", ConfigFiles: &files}).Error; err != nil {
		t.Fatal(err)
	}

	config, err := ReadRollbackConfig(db, watcher.ID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	got := config.Services[0]
	if got.SnapshotEnv != "OLD=1" || !got.HasDraft || got.DraftEnv != "NEW=1" || got.ServiceName != "api-svc" {
		t.Fatalf("unexpected rollback config: %+v", got)
	}

	applied, finish, err := StageRollbackDrafts(db, watcher.ID, "v1")
	if err != nil || applied != 1 {
		t.Fatalf("stage: applied=%d err=%v", applied, err)
	}
	if env := snapshotEnv(t, db, watcher.ID, "v1"); env != "NEW=1" {
		t.Fatalf("staged snapshot env = %q, want NEW=1", env)
	}
	failure := errors.New("health check failed")
	if err := finish(failure); !errors.Is(err, failure) {
		t.Fatalf("finish lost the rollback error: %v", err)
	}
	if env := snapshotEnv(t, db, watcher.ID, "v1"); env != "OLD=1" {
		t.Fatalf("failed rollback left snapshot env %q, want OLD=1", env)
	}
	var drafts int64
	db.Model(&database.ServiceConfigRevision{}).Count(&drafts)
	if drafts != 1 {
		t.Fatalf("failed rollback consumed its draft: drafts=%d", drafts)
	}
}

func TestStageRollbackDraftsConsumesOnSuccess(t *testing.T) {
	db, watcher, service := newRollbackDraftFixture(t)
	if err := db.Create(&database.ServiceConfigRevision{ServiceID: service.ID, TargetVersion: "v1", EnvContent: "NEW=1"}).Error; err != nil {
		t.Fatal(err)
	}
	_, finish, err := StageRollbackDrafts(db, watcher.ID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if env := snapshotEnv(t, db, watcher.ID, "v1"); env != "NEW=1" {
		t.Fatalf("successful rollback snapshot env = %q, want NEW=1", env)
	}
	config, err := ReadRollbackConfig(db, watcher.ID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if config.Source != SnapshotSourceRollbackEdit || config.Services[0].HasDraft {
		t.Fatalf("draft not consumed or source not recorded: %+v", config)
	}
}

func TestStageRollbackDraftsIgnoresNextAndRejectsUnsafeFiles(t *testing.T) {
	db, watcher, service := newRollbackDraftFixture(t)
	if err := db.Create(&database.ServiceConfigRevision{ServiceID: service.ID, TargetVersion: "next", EnvContent: "NEXT=1"}).Error; err != nil {
		t.Fatal(err)
	}
	applied, finish, err := StageRollbackDrafts(db, watcher.ID, "v1")
	if err != nil || applied != 0 {
		t.Fatalf("next draft applied to rollback: applied=%d err=%v", applied, err)
	}
	_ = finish(nil)
	if versions, err := RollbackDraftVersions(db, watcher.ID); err != nil || len(versions) != 0 {
		t.Fatalf("next listed as a rollback draft: %v %v", versions, err)
	}

	unsafe := []database.RevisionConfigFile{{FilePath: "../escape.json", Target: "app_dir", Content: "{}"}}
	if err := db.Create(&database.ServiceConfigRevision{ServiceID: service.ID, TargetVersion: "v1", EnvContent: "NEW=1", ConfigFiles: &unsafe}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := StageRollbackDrafts(db, watcher.ID, "v1"); err == nil {
		t.Fatal("unsafe draft file was staged")
	}
	if env := snapshotEnv(t, db, watcher.ID, "v1"); env != "OLD=1" {
		t.Fatalf("rejected draft changed the snapshot: %q", env)
	}
	if versions, _ := RollbackDraftVersions(db, watcher.ID); len(versions) != 1 || versions[0] != "v1" {
		t.Fatalf("draft versions = %v, want [v1]", versions)
	}
}
