package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
)

func TestFeedSourcesResolvePinnedRelease(t *testing.T) {
	manifest := `{"services":{"api":{"version":"services/api/v1.4.0","artifact_url":"api.zip"}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feed/releases/services/api/v1.4.0/version.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(manifest))
	}))
	defer server.Close()
	dir := t.TempDir()
	releaseDir := filepath.Join(dir, "releases", "services", "api", "v1.4.0")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, feedManifestName), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		source feedSource
		want   string
	}{
		{name: "http", source: newHTTPFeedSource(server.URL+"/feed", FeedAuth{}, newTestLogger()), want: server.URL + "/feed/releases/services/api/v1.4.0/api.zip"},
		{name: "fs", source: newFSFeedSource(dir, newTestLogger()), want: filepath.Join(releaseDir, "api.zip")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Relative artifact paths in a per-release manifest resolve beside it.
			meta, err := tt.source.FetchMetadata(context.Background(), "services/api/v1.4.0", "api")
			if err != nil {
				t.Fatal(err)
			}
			if got := meta.Services["api"].ArtifactURL; got != tt.want {
				t.Fatalf("artifact = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFeedCatalogPagesReleasesForTheWatcherService(t *testing.T) {
	feed := t.TempDir()
	index := feedIndex{}
	for i := 35; i >= 1; i-- {
		tag := fmt.Sprintf("v1.%d.0", i)
		index.Releases = append(index.Releases, feedRelease{Tag: tag, Services: map[string]ServiceMeta{
			"api": {Version: tag, Artifact: "api.zip", ArtifactURL: "releases/" + tag + "/api.zip"},
		}})
	}
	// Monorepo feeds also list releases of other services.
	index.Releases = append([]feedRelease{{Tag: "worker/v9", Services: map[string]ServiceMeta{"worker": {Version: "v9", ArtifactURL: "releases/worker/v9/worker.zip"}}}}, index.Releases...)
	data, _ := json.Marshal(index)
	if err := os.WriteFile(filepath.Join(feed, feedIndexName), data, 0644); err != nil {
		t.Fatal(err)
	}
	catalog, err := NewFeedCatalog(&database.Watcher{ServiceName: "api", SourceType: SourceFS, MetadataURL: feed}, newTestLogger())
	if err != nil {
		t.Fatal(err)
	}

	first, err := catalog.ListReleaseCatalog(context.Background(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := catalog.ListReleaseCatalog(context.Background(), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Releases) != 30 || !first.HasNext || first.Releases[0].TagName != "v1.35.0" || len(second.Releases) != 5 || second.HasNext {
		t.Fatalf("pages: %d/%v first=%s, %d/%v", len(first.Releases), first.HasNext, first.Releases[0].TagName, len(second.Releases), second.HasNext)
	}
	release := second.Releases[4]
	if release.TagName != "v1.1.0" || release.ID != feedID("v1.1.0") || release.Assets[0].BrowserDownloadURL != filepath.Join(feed, "releases", "v1.1.0", "api.zip") {
		t.Fatalf("release = %+v", release)
	}
	got, asset, version, err := catalog.ResolveCatalogAsset(context.Background(), nil, release.ID, release.Assets[0].ID)
	if err != nil || got.TagName != "v1.1.0" || asset.Name != "api.zip" || version != "v1.1.0" {
		t.Fatalf("resolve = %+v %+v %q %v", got, asset, version, err)
	}
	if _, _, _, err := catalog.ResolveCatalogAsset(context.Background(), nil, release.ID, release.Assets[0].ID+1); err == nil {
		t.Fatal("asset from another release accepted")
	}
}

func TestFeedCatalogDeployRunsStagedArtifact(t *testing.T) {
	installNSSMCommandMock(t)
	r, svc := candidateFixture(t)
	if err := r.db.AutoMigrate(&database.CatalogArtifact{}); err != nil {
		t.Fatal(err)
	}
	if err := r.db.Model(&svc).UpdateColumn("service_type", "static").Error; err != nil {
		t.Fatal(err)
	}
	feed := t.TempDir()
	var watcher database.Watcher
	r.db.First(&watcher, r.watcherID)
	watcher.SourceType, watcher.MetadataURL, watcher.ServiceName, watcher.Paused = SourceFS, feed, "api", true
	r.db.Save(&watcher)
	now := time.Now().UTC()
	artifact := database.CatalogArtifact{WatcherID: watcher.ID, ServiceName: "api", Repository: feed, InstallDir: watcher.InstallDir, Tag: "v2", Version: "v2", AssetName: "api.zip", ArtifactURL: filepath.Join(feed, "releases", "v2", "api.zip"), DownloadedAt: &now}
	r.db.Create(&artifact)
	path := CatalogArtifactPath(&artifact)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	createTestZipFile(t, path, map[string]string{"index.html": "pinned feed release"})
	artifact.SHA256, _ = fileSHA256(path)
	r.db.Save(&artifact)
	r.db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2", "status": "approved"})

	attempt := queueCatalogTestAttempt(t, r, "v2")
	if err := r.RunCatalog(WithTrace(context.Background(), NewPollTrace("", "manual")), artifact.ID, attempt.ID); err != nil {
		t.Fatal(err)
	}
	r.db.First(&watcher, watcher.ID)
	r.db.First(&attempt, attempt.ID)
	data, err := os.ReadFile(filepath.Join(watcher.InstallDir, "current", "index.html"))
	if watcher.CurrentVersion != "v2" || err != nil || string(data) != "pinned feed release" {
		t.Fatalf("version=%q current=%q %v", watcher.CurrentVersion, data, err)
	}
	if !strings.Contains(attempt.Logs, "github_deployment: skipped for fs release source") {
		t.Fatalf("deploy log = %q", attempt.Logs)
	}
}
