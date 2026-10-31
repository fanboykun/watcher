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

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const testSum = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" // sha256("test")

func TestNormalizeSHA256(t *testing.T) {
	tests := []struct {
		raw, want string
		wantErr   bool
	}{
		{raw: "", want: ""},
		{raw: testSum, want: testSum},
		{raw: "  " + strings.ToUpper(testSum) + " ", want: testSum},
		{raw: "sha256:" + testSum, want: testSum},
		{raw: testSum[:63], wantErr: true},
		{raw: "z" + testSum[1:], wantErr: true},
		{raw: "sha512:" + testSum, wantErr: true},
	}
	for _, tt := range tests {
		got, err := normalizeSHA256(tt.raw)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("normalizeSHA256(%q) = %q, %v; want %q, err=%v", tt.raw, got, err, tt.want, tt.wantErr)
		}
	}
}

// newChecksumFeedWatcher serves one release whose manifest declares sum.
func newChecksumFeedWatcher(t *testing.T, sum string) (*RepoWatcher, *gorm.DB, uint) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			_ = json.NewEncoder(w).Encode(VersionMetadata{Services: map[string]ServiceMeta{
				"api": {Version: "v2", ArtifactURL: "releases/v2/api.zip", SHA256: sum},
			}})
		case "/releases/v2/api.zip":
			_, _ = w.Write([]byte("tampered"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
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
		DownloadRetries: 1, MaxKeptVersions: 3, AutoDeploy: true,
	}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	rw := NewRepoWatcher(&watcher, db, &config.AppConfig{}, newTestLogger(), nil, nil)
	return rw, db, watcher.ID
}

func TestRepoWatcherRun_RejectsArtifactBeforeExtraction(t *testing.T) {
	tests := []struct {
		name, sum, wantErr string
	}{
		{name: "mismatch", sum: testSum, wantErr: "artifact sha256 mismatch: release metadata expects " + testSum},
		{name: "malformed", sum: "not-a-checksum", wantErr: `invalid sha256 "not-a-checksum"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rw, db, watcherID := newChecksumFeedWatcher(t, tt.sum)

			if err := rw.Run(context.Background()); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Run error = %v, want %q", err, tt.wantErr)
			}
			var attempt database.DeployLog
			if err := db.Where("watcher_id = ?", watcherID).First(&attempt).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.Status != "failed" || !strings.Contains(attempt.Error, tt.wantErr) {
				t.Fatalf("attempt status=%q error=%q", attempt.Status, attempt.Error)
			}
			// A bad artifact is a real failure: it counts toward the per-version retry cap.
			if got := rw.state.ConsecutiveFailuresForVersion("v2"); got != 1 {
				t.Fatalf("consecutive failures = %d, want 1", got)
			}
			entries, _ := os.ReadDir(filepath.Join(rw.wcfg.InstallDir, "downloads"))
			if len(entries) != 0 {
				t.Fatalf("rejected artifact left in downloads: %v", entries)
			}
			if _, err := os.Stat(filepath.Join(rw.wcfg.InstallDir, "releases")); !os.IsNotExist(err) {
				t.Fatalf("release dir created for a rejected artifact: %v", err)
			}
		})
	}
}

func TestVerifyArtifactChecksumWithoutManifestSum(t *testing.T) {
	rw, db, watcherID := newChecksumFeedWatcher(t, "")
	deployLogID, err := rw.state.SetDeploying("v2", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "api.zip")
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := rw.verifyArtifactChecksum(path, ""); err != nil {
		t.Fatal(err)
	}
	if err := rw.verifyArtifactChecksum(path, testSum); err != nil {
		t.Fatalf("matching sum rejected: %v", err)
	}
	var attempt database.DeployLog
	if err := db.Where("watcher_id = ? AND id = ?", watcherID, deployLogID).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attempt.Logs, "integrity not verified") || !strings.Contains(attempt.Logs, "sha256 verified "+testSum) {
		t.Fatalf("deploy log = %q", attempt.Logs)
	}
}

func TestBuildMetadataFromReleaseUsesGitHubDigest(t *testing.T) {
	meta := buildMetadataFromRelease(&githubRelease{TagName: "v1", Assets: []githubAsset{
		{Name: "api-windows-amd64.zip", Digest: "sha256:" + testSum},
		{Name: "worker-windows-amd64.zip"},
	}})
	if got := meta.Services["api"].SHA256; got != "sha256:"+testSum {
		t.Fatalf("api sha256 = %q", got)
	}
	if got := meta.Services["worker"].SHA256; got != "" {
		t.Fatalf("worker sha256 = %q, want empty", got)
	}
}

func TestCatalogManifestChecksumBindsAssetDigest(t *testing.T) {
	tests := []struct {
		name, githubDigest, wantDigest, wantErr string
	}{
		{name: "manifest fills missing digest", wantDigest: "sha256:" + testSum},
		{name: "matching digests", githubDigest: "sha256:" + strings.ToUpper(testSum), wantDigest: "sha256:" + testSum},
		{name: "conflicting digests", githubDigest: "sha256:" + strings.Repeat("0", 64), wantErr: "does not match the GitHub asset digest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/repos/owner/repo/releases/42":
					fmt.Fprintf(w, `{"id":42,"tag_name":"v2","assets":[{"id":7,"name":"api.zip","digest":%q}]}`, tt.githubDigest)
				case "/repos/owner/repo/releases/tags/v2":
					fmt.Fprintf(w, `{"tag_name":"v2","assets":[{"id":9,"name":"version.json","browser_download_url":%q}]}`, server.URL+"/manifest")
				case "/manifest":
					fmt.Fprintf(w, `{"services":{"api":{"version":"v2","artifact":"api.zip","sha256":%q}}}`, testSum)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := newGitHubClient("", newTestLogger(), newGitHubBucket())
			client.apiBase = server.URL
			w := &database.Watcher{MetadataURL: "https://github.com/owner/repo/releases/latest/download/version.json", ServiceName: "api"}

			_, asset, _, err := client.ResolveCatalogAsset(context.Background(), w, 42, 7)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || asset.Digest != tt.wantDigest {
				t.Fatalf("digest = %q, err = %v; want %q", asset.Digest, err, tt.wantDigest)
			}
		})
	}
}
