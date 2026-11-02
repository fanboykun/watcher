package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newReleaseMarkDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.ReleaseMark{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBlockReleaseRoundTripKeepsNotes(t *testing.T) {
	db := newReleaseMarkDB(t)
	published := time.Date(2026, 10, 31, 4, 19, 5, 0, time.UTC)
	notes := &ReleaseNotes{Tag: "v2", Title: "Faster sync", Notes: "Fixes", URL: "https://github.com/o/r/releases/tag/v2", PublishedAt: &published}
	if err := RecordReleaseNotes(db, 1, "v2", notes); err != nil {
		t.Fatal(err)
	}
	if err := CheckReleaseAllowed(db, 1, "v2"); err != nil {
		t.Fatalf("unblocked release refused: %v", err)
	}

	if _, err := BlockRelease(db, 1, "v2", "corrupts invoices"); err != nil {
		t.Fatal(err)
	}
	err := CheckReleaseAllowed(db, 1, "v2")
	var blocked *ReleaseBlockedError
	if !errors.Is(err, ErrReleaseBlocked) || !errors.As(err, &blocked) || blocked.Reason != "corrupts invoices" {
		t.Fatalf("blocked release allowed or reason lost: %v", err)
	}
	if err := CheckReleaseAllowed(db, 2, "v2"); err != nil {
		t.Fatalf("block leaked to another watcher: %v", err)
	}

	// Later polls refresh notes without lifting the block.
	notes.Notes = "Fixes, edited"
	if err := RecordReleaseNotes(db, 1, "v2", notes); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(CheckReleaseAllowed(db, 1, "v2"), ErrReleaseBlocked) {
		t.Fatal("recording notes lifted the block")
	}

	mark, err := UnblockRelease(db, 1, "v2")
	if err != nil {
		t.Fatal(err)
	}
	if mark.BlockedAt != nil || mark.Notes != "Fixes, edited" || mark.Title != "Faster sync" {
		t.Fatalf("unexpected mark after unblock: %+v", mark)
	}
	if err := CheckReleaseAllowed(db, 1, "v2"); err != nil {
		t.Fatalf("unblocked release refused: %v", err)
	}
}

func TestBlockReleaseBeforeWatcherSeesIt(t *testing.T) {
	db := newReleaseMarkDB(t)
	if _, err := BlockRelease(db, 1, "v9", "known bad build"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(CheckReleaseAllowed(db, 1, "v9"), ErrReleaseBlocked) {
		t.Fatal("pre-emptive block was not enforced")
	}
	if err := RecordReleaseNotes(db, 1, "v9", &ReleaseNotes{Title: "v9"}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(CheckReleaseAllowed(db, 1, "v9"), ErrReleaseBlocked) {
		t.Fatal("first notes for a pre-blocked version lifted the block")
	}
}

func TestNotesFromGitHubReleaseSanitizes(t *testing.T) {
	notes := NotesFromGitHubRelease(&CatalogRelease{
		TagName:     " v2 ",
		Name:        "Release",
		Body:        strings.Repeat("a", releaseNotesLimit+10),
		HTMLURL:     "javascript:alert(1)",
		PublishedAt: "2026-10-31T04:19:05Z",
	})
	if notes.URL != "" {
		t.Fatalf("unsafe URL kept: %q", notes.URL)
	}
	if notes.Tag != "v2" || notes.PublishedAt == nil {
		t.Fatalf("tag or publish time not parsed: %+v", notes)
	}
	if len(notes.Notes) > releaseNotesLimit+10 || !strings.HasSuffix(notes.Notes, "…") {
		t.Fatalf("notes not truncated: len=%d", len(notes.Notes))
	}
	if got := NotesFromGitHubRelease(&CatalogRelease{HTMLURL: "https://github.com/o/r/releases/tag/v2"}).URL; got != "https://github.com/o/r/releases/tag/v2" {
		t.Fatalf("release URL dropped: %q", got)
	}
}

func TestBuildMetadataFromReleaseCarriesNotes(t *testing.T) {
	meta := buildMetadataFromRelease(&githubRelease{
		TagName: "v2", Name: "Second", Body: "notes", HTMLURL: "https://github.com/o/r/releases/tag/v2",
		Assets: []githubAsset{{Name: "api-v2.zip"}},
	})
	if meta.Release == nil || meta.Release.Title != "Second" || meta.Release.Notes != "notes" {
		t.Fatalf("release notes missing from metadata: %+v", meta.Release)
	}
}

func TestCleanOldReleasesKeepsBlockedOutsideTheLimit(t *testing.T) {
	dir := t.TempDir()
	for i, v := range []string{"v1", "v2", "v3", "v4"} {
		path := filepath.Join(dir, "releases", v)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(path, modTime, modTime)
	}

	// v4 is newest but blocked, so v3 and v2 fill the two slots.
	if err := CleanOldReleases(dir, 2, func(v string) bool { return v == "v4" }); err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]bool{"v4": true, "v3": true, "v2": true, "v1": false} {
		_, err := os.Stat(filepath.Join(dir, "releases", v))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists=%v, want %v", v, exists, want)
		}
	}
}

func TestResolveRollbackVersionSkipsBlockedPrevious(t *testing.T) {
	dir := t.TempDir()
	for i, v := range []string{"v1", "v2", "v3"} {
		path := filepath.Join(dir, "releases", releaseStorageName(v))
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		d := NewDeployer(&WatcherConfig{InstallDir: dir}, "nssm.exe", newTestLogger(), func(string) {})
		if err := d.captureConfigSnapshot(v, SnapshotSourceDeployment); err != nil {
			t.Fatal(err)
		}
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(path, modTime, modTime)
	}
	blocked := map[string]bool{"v2": true}
	d := NewDeployer(&WatcherConfig{InstallDir: dir}, "nssm.exe", newTestLogger(), func(string) {})
	d.blocked = func(v string) bool { return blocked[v] }

	// Deploying v3 from blocked v2 must restore v1, not the blocked release.
	if got := d.resolveRollbackVersion("v3", "v2"); got != "v1" {
		t.Fatalf("resolveRollbackVersion = %q, want v1", got)
	}
	blocked["v1"] = true
	if got := d.resolveRollbackVersion("v3", "v2"); got != "" {
		t.Fatalf("resolveRollbackVersion = %q, want no rollback when every retained release is blocked", got)
	}
}

func newBlockedTargetServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var downloads atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version.json" {
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{
				"api": {Version: "v2", ArtifactURL: server.URL + "/artifact.zip"},
			}})
			return
		}
		downloads.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server, &downloads
}

func TestRepoWatcherRunSkipsBlockedTarget(t *testing.T) {
	server, downloads := newBlockedTargetServer(t)
	rw, db, watcherID := newRateLimitedWatcher(t, server)
	if _, err := BlockRelease(db, watcherID, "v2", "data loss"); err != nil {
		t.Fatal(err)
	}

	if err := rw.Run(context.Background()); err != nil {
		t.Fatalf("blocked target should not fail the poll: %v", err)
	}
	var deploys int64
	db.Model(&database.DeployLog{}).Count(&deploys)
	if deploys != 0 || downloads.Load() != 0 {
		t.Fatalf("blocked target was deployed: deploys=%d downloads=%d", deploys, downloads.Load())
	}
	var poll database.PollEvent
	if err := db.Where("watcher_id = ?", watcherID).Order("id desc").First(&poll).Error; err != nil {
		t.Fatal(err)
	}
	if poll.Status != "blocked" || poll.RemoteVersion != "v2" || !strings.Contains(poll.Error, "data loss") {
		t.Fatalf("unexpected poll event: %+v", poll)
	}
}

func TestRepoWatcherRunClosesRedeployOfBlockedTarget(t *testing.T) {
	server, downloads := newBlockedTargetServer(t)
	rw, db, watcherID := newRateLimitedWatcher(t, server)
	installDir := rw.wcfg.InstallDir
	if err := os.MkdirAll(filepath.Join(installDir, "releases", "v1"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(installDir, "releases", "v1"), filepath.Join(installDir, "current")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// A queued manual redeploy clears current_version to force the next poll to deploy.
	now := time.Now().UTC()
	redeploy := database.DeployLog{WatcherID: watcherID, TriggeredBy: "manual", Kind: "deploy", Reason: "manual_redeploy", Version: "v1", Status: "in_progress", StartedAt: &now}
	if err := db.Create(&redeploy).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.Watcher{}).Where("id = ?", watcherID).UpdateColumn("current_version", "").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := BlockRelease(db, watcherID, "v2", "data loss"); err != nil {
		t.Fatal(err)
	}

	if err := rw.Run(context.Background()); err != nil {
		t.Fatalf("blocked redeploy should not fail the poll: %v", err)
	}
	if downloads.Load() != 0 {
		t.Fatal("blocked target was downloaded")
	}
	if err := db.First(&redeploy, redeploy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if redeploy.Status != "blocked" || redeploy.CompletedAt == nil || redeploy.Version != "v2" {
		t.Fatalf("queued redeploy not closed as blocked: %+v", redeploy)
	}
	var watcher database.Watcher
	if err := db.First(&watcher, watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.CurrentVersion != "v1" || watcher.Status != string(StatusHealthy) {
		t.Fatalf("watcher state not restored: current=%q status=%q", watcher.CurrentVersion, watcher.Status)
	}
}
