package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// conditionalFeed serves version.json with an ETag and records each request's
// If-None-Match header.
type conditionalFeed struct {
	mu          sync.Mutex
	etag        string
	conditional []string
	full        int
}

func (f *conditionalFeed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/version.json" {
		http.NotFound(w, r)
		return
	}
	f.conditional = append(f.conditional, r.Header.Get("If-None-Match"))
	if f.etag != "" && r.Header.Get("If-None-Match") == f.etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	f.full++
	if f.etag != "" {
		w.Header().Set("ETag", f.etag)
	}
	_, _ = w.Write([]byte(`{"services":{"api":{"version":"v2","artifact_url":"releases/v2/api.zip"}}}`))
}

func TestHTTPFeedSource_RevalidatesWithETag(t *testing.T) {
	feed := &conditionalFeed{etag: `"v2"`}
	server := httptest.NewServer(feed)
	defer server.Close()
	source := newHTTPFeedSource(server.URL, FeedAuth{}, newTestLogger())
	source.setPollCache(newFeedPollCache())
	scheduled := WithTrace(context.Background(), NewPollTrace("", "scheduled"))

	first, err := source.FetchMetadata(scheduled, "latest", "api")
	if err != nil || first.notModified {
		t.Fatalf("first poll: notModified=%v err=%v", first != nil && first.notModified, err)
	}
	second, err := source.FetchMetadata(scheduled, "latest", "api")
	if err != nil || !second.notModified || second.Services["api"].ArtifactURL != server.URL+"/releases/v2/api.zip" {
		t.Fatalf("second poll: %+v err=%v", second, err)
	}
	// A manual check always rereads the manifest.
	manual, err := source.FetchMetadata(WithTrace(context.Background(), NewPollTrace("", "manual")), "latest", "api")
	if err != nil || manual.notModified {
		t.Fatalf("manual poll: notModified=%v err=%v", manual != nil && manual.notModified, err)
	}
	if got := strings.Join(feed.conditional, ","); got != `,"v2",` || feed.full != 2 {
		t.Fatalf("If-None-Match sequence = %q, full fetches = %d", got, feed.full)
	}
}

func TestHTTPFeedSource_WithoutValidatorsAlwaysFetches(t *testing.T) {
	feed := &conditionalFeed{}
	server := httptest.NewServer(feed)
	defer server.Close()
	source := newHTTPFeedSource(server.URL, FeedAuth{}, newTestLogger())
	source.setPollCache(newFeedPollCache())
	for range 3 {
		meta, err := source.FetchMetadata(context.Background(), "latest", "api")
		if err != nil || meta.notModified {
			t.Fatalf("poll: %+v %v", meta, err)
		}
	}
	if feed.full != 3 {
		t.Fatalf("full fetches = %d, want 3", feed.full)
	}
}

func TestFSFeedSource_ReusesUnchangedManifest(t *testing.T) {
	root := t.TempDir()
	writeFeedManifest(t, root, map[string]ServiceMeta{"api": {Version: "v1", ArtifactURL: "releases/v1/api.zip"}})
	source := newFSFeedSource(root, newTestLogger())
	source.setPollCache(newFeedPollCache())

	if meta, err := source.FetchMetadata(context.Background(), "latest", "api"); err != nil || meta.notModified {
		t.Fatalf("first read: %+v %v", meta, err)
	}
	meta, err := source.FetchMetadata(context.Background(), "latest", "api")
	if err != nil || !meta.notModified || meta.Services["api"].Version != "v1" {
		t.Fatalf("unchanged read: %+v %v", meta, err)
	}
	writeFeedManifest(t, root, map[string]ServiceMeta{"api": {Version: "v2", ArtifactURL: "releases/v2/api.zip"}})
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(root, feedManifestName), later, later); err != nil {
		t.Fatal(err)
	}
	meta, err = source.FetchMetadata(context.Background(), "latest", "api")
	if err != nil || meta.notModified || meta.Services["api"].Version != "v2" {
		t.Fatalf("changed read: %+v %v", meta, err)
	}
}

func newConditionalFeedWatcher(t *testing.T, server *httptest.Server, current string) (*RepoWatcher, *gorm.DB, uint) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}, &database.PollEvent{}, &database.ServiceConfigRevision{}); err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{
		Name: "alpha", ServiceName: "api", SourceType: SourceHTTP, MetadataURL: server.URL, InstallDir: t.TempDir(),
		DownloadRetries: 1, MaxKeptVersions: 3, AutoDeploy: true, CurrentVersion: current, Status: string(StatusHealthy),
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	return NewRepoWatcher(&watcher, db, &config.AppConfig{}, newTestLogger(), nil, nil), db, watcher.ID
}

func TestRepoWatcherRun_RecordsUnchangedFeedPolls(t *testing.T) {
	feed := &conditionalFeed{etag: `"v2"`}
	server := httptest.NewServer(feed)
	defer server.Close()
	rw, db, watcherID := newConditionalFeedWatcher(t, server, "v2")

	for range 2 {
		if err := rw.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	var statuses []string
	db.Model(&database.PollEvent{}).Where("watcher_id = ?", watcherID).Order("id").Pluck("status", &statuses)
	if strings.Join(statuses, ",") != "no_update,unchanged" || feed.full != 1 {
		t.Fatalf("statuses = %v, full fetches = %d", statuses, feed.full)
	}
}

func TestRepoWatcherRun_UnchangedFeedStillRetriesPendingDeploy(t *testing.T) {
	// The artifact is missing, so each deploy fails; a 304 must not hide that
	// the watcher is still behind the feed.
	feed := &conditionalFeed{etag: `"v2"`}
	server := httptest.NewServer(feed)
	defer server.Close()
	rw, db, watcherID := newConditionalFeedWatcher(t, server, "v1")

	for range 2 {
		_ = rw.Run(context.Background())
	}
	var attempts int64
	db.Model(&database.DeployLog{}).Where("watcher_id = ? AND status = ?", watcherID, "failed").Count(&attempts)
	if attempts != 2 || feed.full != 1 {
		t.Fatalf("failed attempts = %d, full fetches = %d; want 2 attempts from 1 fetch", attempts, feed.full)
	}
}
