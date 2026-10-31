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

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeFeedRoot(t *testing.T) {
	tests := []struct {
		raw, want, wantErr string
	}{
		{raw: "https://releases.internal/apps/api", want: "https://releases.internal/apps/api"},
		{raw: " https://releases.internal/apps/api/ ", want: "https://releases.internal/apps/api"},
		{raw: "https://releases.internal/apps/api/version.json", want: "https://releases.internal/apps/api"},
		{raw: "http://10.0.0.5:9000/bucket/version.json", want: "http://10.0.0.5:9000/bucket"},
		{raw: "https://releases.internal", want: "https://releases.internal"},
		{raw: "ftp://releases.internal/api", wantErr: "http:// or https://"},
		{raw: "releases.internal/api", wantErr: "http:// or https://"},
		{raw: "https://user:pass@releases.internal/api", wantErr: "credentials"},
		{raw: "https://releases.internal/api?sig=abc", wantErr: "query string"},
	}
	for _, tt := range tests {
		got, err := NormalizeFeedRoot(tt.raw)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("NormalizeFeedRoot(%q) error=%v, want %q", tt.raw, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("NormalizeFeedRoot(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
		}
	}
}

func TestHTTPFeedSource_FetchMetadataResolvesArtifactURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("feed request carried Authorization header")
		}
		if r.URL.Path != "/feeds/app/version.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"services":{
			"relative":{"version":"v1.2.0","artifact_url":"releases/v1.2.0/app.zip"},
			"rooted":{"version":"v1.2.0","artifact_url":"/mirror/app.zip"},
			"absolute":{"version":"v1.2.0","artifact_url":"https://cdn.internal/app.zip"},
			"slashed":{"version":"services/prs/v0.1.0","artifact_url":"releases/services/prs/v0.1.0/prs.zip"}
		}}`))
	}))
	defer server.Close()

	meta, err := newHTTPFeedSource(server.URL+"/feeds/app/", FeedAuth{}, newTestLogger()).FetchMetadata(context.Background(), "", "relative")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"relative": server.URL + "/feeds/app/releases/v1.2.0/app.zip",
		"rooted":   server.URL + "/mirror/app.zip",
		"absolute": "https://cdn.internal/app.zip",
		"slashed":  server.URL + "/feeds/app/releases/services/prs/v0.1.0/prs.zip",
	}
	for name, url := range want {
		if got := meta.Services[name].ArtifactURL; got != url {
			t.Errorf("%s artifact_url = %q, want %q", name, got, url)
		}
	}
}

func TestHTTPFeedSource_FetchMetadataErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad-url/version.json":
			_, _ = w.Write([]byte(`{"services":{"api":{"version":"v1","artifact_url":"file:///etc/passwd"}}}`))
		case "/broken/version.json":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tests := []struct {
		name, root, ref, wantErr string
	}{
		{name: "ref escaping releases dir", root: server.URL + "/app", ref: "../v1.2.0", wantErr: "must be a tag"},
		{name: "missing manifest", root: server.URL + "/missing", wantErr: "HTTP 404"},
		{name: "server error", root: server.URL + "/broken", wantErr: "HTTP 500"},
		{name: "non-http artifact", root: server.URL + "/bad-url", wantErr: "must resolve to an http(s) URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHTTPFeedSource(tt.root, FeedAuth{}, newTestLogger()).FetchMetadata(context.Background(), tt.ref, "api")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPFeedSource_DownloadRetriesTransientFailures(t *testing.T) {
	original := sleepContext
	sleepContext = func(context.Context, time.Duration) error { return nil }
	t.Cleanup(func() { sleepContext = original })

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "app.zip")
	if err := newHTTPFeedSource(server.URL, FeedAuth{}, newTestLogger()).DownloadArtifact(context.Background(), server.URL+"/app.zip", dest, 3); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); string(got) != "zip-bytes" || hits.Load() != 2 {
		t.Fatalf("artifact=%q hits=%d, want zip-bytes after 2 hits", got, hits.Load())
	}
	if _, err := os.Stat(dest + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file left behind: %v", err)
	}
}

func TestRepoWatcherRun_HTTPFeedDeploysWithoutGitHub(t *testing.T) {
	installNSSMCommandMock(t)
	root := t.TempDir()
	artifactPath := filepath.Join(root, "artifact.zip")
	createTestZipFile(t, artifactPath, map[string]string{"api.exe": "target"})
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := fileSHA256(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("GitHub token sent to feed host: %s %s", r.Method, r.URL.Path)
		}
		switch r.URL.Path {
		case "/feeds/api/version.json":
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{
				"api": {Version: "v2", Artifact: "api.zip", ArtifactURL: "releases/v2/api.zip", SHA256: strings.ToUpper(sum)},
			}})
		case "/feeds/api/releases/v2/api.zip":
			_, _ = w.Write(artifact)
		default:
			t.Errorf("unexpected feed request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.DeployLog{}, &database.PollEvent{}, &database.ServiceConfigRevision{}); err != nil {
		t.Fatal(err)
	}
	dbWatcher := database.Watcher{
		Name: "alpha", ServiceName: "api", SourceType: SourceHTTP, MetadataURL: server.URL + "/feeds/api",
		InstallDir: root, CheckIntervalSec: 60, DownloadRetries: 1, MaxKeptVersions: 3, AutoDeploy: true,
	}
	if err := db.Create(&dbWatcher).Error; err != nil {
		t.Fatal(err)
	}
	// A configured GitHub token and reporting must not reach a feed watcher.
	global := &config.AppConfig{Environment: "production", GitHubToken: "configured-token", GitHubDeployEnabled: true}
	rw := NewRepoWatcher(&dbWatcher, db, global, newTestLogger(), nil, nil)
	rw.github = NewGitHubPool()
	rw.wcfg.Services = []ServiceConfig{{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"}}
	rw.deployer = NewDeployer(rw.wcfg, "nssm.exe", rw.log, rw.state.AppendDeployLog)
	runner := newScriptedNSSMRunner(t,
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{args: "start api", output: string(ServiceStateStartPending)},
		stateStep("api", ServiceStateStartPending),
		stateStep("api", ServiceStateRunning),
	)
	rw.deployer.serviceManager = newTestServiceManager(runner.run)

	if err := rw.Run(context.Background()); err != nil {
		t.Fatalf("feed poll failed: %v", err)
	}
	runner.assertDone()

	var got database.Watcher
	if err := db.First(&got, dbWatcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.CurrentVersion != "v2" || got.Status != string(StatusHealthy) {
		t.Fatalf("watcher version=%q status=%q, want healthy v2", got.CurrentVersion, got.Status)
	}
	var attempt database.DeployLog
	if err := db.Where("watcher_id = ?", dbWatcher.ID).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.Status != "succeeded" || !strings.Contains(attempt.Logs, "github_deployment: skipped for http release source") || !strings.Contains(attempt.Logs, "artifact: sha256 verified "+sum) {
		t.Fatalf("attempt status=%q logs=%q", attempt.Status, attempt.Logs)
	}
	if _, err := ReadReleaseDeploymentMetadata(root, "v2"); err == nil {
		t.Fatal("feed deploy must not record GitHub deployment metadata")
	}
}
