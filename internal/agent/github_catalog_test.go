package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
)

type catalogRoundTripper func(*http.Request) (*http.Response, error)

func (f catalogRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type catalogCancelReader struct {
	reader io.Reader
	cancel context.CancelFunc
	atEOF  bool
}

func (r *catalogCancelReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if !r.atEOF || err == io.EOF {
		r.cancel()
		if !r.atEOF {
			return n, context.Canceled
		}
	}
	return n, err
}

func TestCatalogDownloadValidationAndCancellationCleanup(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	f, err := archive.Create("api.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("selected")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, digest           string
		invalid, cancel, atEOF bool
	}{
		{name: "invalid ZIP", invalid: true},
		{name: "digest mismatch", digest: "sha256:wrong"},
		{name: "cancel during body", cancel: true},
		{name: "cancel at EOF", cancel: true, atEOF: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var body io.Reader = bytes.NewReader(buffer.Bytes())
			if tc.invalid {
				body = strings.NewReader("not a ZIP")
			}
			if tc.cancel {
				body = &catalogCancelReader{reader: body, cancel: cancel, atEOF: tc.atEOF}
			}
			client := NewGitHubClient("", newTestLogger())
			client.client.Transport = catalogRoundTripper(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
			})
			dir := t.TempDir()
			path := filepath.Join(dir, "artifact.zip")
			_, err := client.DownloadCatalogAsset(ctx, "https://github.com/owner/repo", 7, path, tc.digest)
			if err == nil {
				t.Fatal("failed download accepted")
			}
			if tc.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation lost: %v", err)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("failed download left files: %v %v", files, err)
			}
		})
	}
}

func TestCatalogReleaseEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, tag, assetName, manifest, wantVersion, wantError string
		id, assetID                                            int64
		draft, prerelease                                      bool
	}{
		{name: "older release", id: 42, tag: "v0.1", assetID: 7, assetName: "api.zip", wantVersion: "v0.1"},
		{name: "prerelease", id: 42, tag: "v2-rc1", prerelease: true, assetID: 7, assetName: "api.ZIP", wantVersion: "v2-rc1"},
		{name: "multiple ZIPs", id: 42, tag: "v2", assetID: 8, assetName: "api.zip", wantVersion: "v2"},
		{name: "draft", id: 42, tag: "v2", draft: true, assetID: 7, assetName: "api.zip", wantError: "not published"},
		{name: "wrong release", id: 99, tag: "v2", assetID: 7, assetName: "api.zip", wantError: "not published"},
		{name: "blank tag", id: 42, tag: " ", assetID: 7, assetName: "api.zip", wantError: "not published"},
		{name: "non ZIP", id: 42, tag: "v2", assetID: 7, assetName: "api.tar.gz", wantError: "ZIP release asset"},
		{name: "foreign asset", id: 42, tag: "v2", assetID: 99, assetName: "api.zip", wantError: "does not belong"},
		{name: "manifest service version", id: 42, tag: "release-tag", assetID: 7, assetName: "api.zip", manifest: `{"services":{"api":{"version":"v2.7.1","artifact":"api.zip"}}}`, wantVersion: "v2.7.1"},
		{name: "manifest URL binding", id: 42, tag: "release-tag", assetID: 7, assetName: "api.zip", manifest: `{"services":{"api":{"version":"v2.7.1","artifact_url":"https://github.com/owner/repo/releases/download/release-tag/api.zip"}}}`, wantVersion: "v2.7.1"},
		{name: "manifest other asset", id: 42, tag: "release-tag", assetID: 8, assetName: "api.zip", manifest: `{"services":{"api":{"version":"v2.7.1","artifact":"api.zip"}}}`, wantError: "not the artifact declared"},
		{name: "manifest missing service", id: 42, tag: "release-tag", assetID: 7, assetName: "api.zip", manifest: `{"services":{"other":{"version":"v2.7.1","artifact":"api.zip"}}}`, wantError: "not the artifact declared"},
		{name: "manifest missing version", id: 42, tag: "release-tag", assetID: 7, assetName: "api.zip", manifest: `{"services":{"api":{"artifact":"api.zip"}}}`, wantError: "not the artifact declared"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := NewGitHubClient("", newTestLogger())
			client.client.Transport = catalogRoundTripper(func(r *http.Request) (*http.Response, error) {
				var data []byte
				switch r.URL.Path {
				case "/repos/owner/repo/releases/42":
					data, _ = json.Marshal(CatalogRelease{ID: tc.id, TagName: tc.tag, Draft: tc.draft, Prerelease: tc.prerelease, Assets: []githubAsset{{ID: 7, Name: tc.assetName, BrowserDownloadURL: "https://github.com/owner/repo/releases/download/release-tag/api.zip"}, {ID: 8, Name: "other.zip"}}})
				case "/repos/owner/repo/releases/tags/release-tag":
					data = []byte(`{"tag_name":"release-tag","assets":[{"id":9,"name":"version.json","browser_download_url":"https://github.com/owner/repo/manifest"}]}`)
				case "/owner/repo/manifest":
					data = []byte(tc.manifest)
				default:
					t.Fatalf("unexpected request: %s", r.URL)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
			})
			watcher := &database.Watcher{MetadataURL: "https://github.com/owner/repo", ServiceName: "api"}
			if tc.manifest != "" {
				watcher.MetadataURL += "/releases/latest/download/version.json"
			}
			_, asset, version, err := client.ResolveCatalogAsset(context.Background(), watcher, 42, tc.assetID)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected %q, got %v", tc.wantError, err)
				}
			} else if err != nil || version != tc.wantVersion || asset.ID != tc.assetID {
				t.Fatalf("resolved asset=%+v version=%q err=%v", asset, version, err)
			}
		})
	}
}

func TestCatalogPaginationAndPrivateAssetDownload(t *testing.T) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	f, _ := archive.Create("api.exe")
	_, _ = f.Write([]byte("artifact"))
	_ = archive.Close()
	sum := sha256.Sum256(buffer.Bytes())
	digest := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("watcher token missing")
		}
		switch r.URL.Path {
		case "/repos/owner/repo/releases":
			if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "30" {
				t.Error("pagination was not forwarded")
			}
			w.Header().Set("Link", `<https://api.github.com/next>; rel="next"`)
			fmt.Fprint(w, `[{"id":42,"tag_name":"v2","name":"Release two","body":"Notes","prerelease":true,"assets":[{"id":7,"name":"api.zip"}]}]`)
		case "/repos/owner/repo/releases/42":
			fmt.Fprint(w, `{"id":42,"tag_name":"v2","assets":[{"id":7,"name":"api.zip","browser_download_url":"https://github.com/owner/repo/releases/download/v2/api.zip"}]}`)
		case "/repos/owner/repo/releases/assets/7":
			if r.Header.Get("Accept") != "application/octet-stream" {
				t.Error("asset content header missing")
			}
			_, _ = w.Write(buffer.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewGitHubClient("secret", newTestLogger())
	client.apiBase = server.URL
	catalog, err := client.ListReleaseCatalog(context.Background(), "https://github.com/owner/repo", 2)
	if err != nil || catalog.Page != 2 || !catalog.HasNext || len(catalog.Releases) != 1 || !catalog.Releases[0].Prerelease {
		t.Fatalf("catalog: %+v %v", catalog, err)
	}
	w := &database.Watcher{MetadataURL: "https://github.com/owner/repo", ServiceName: "api"}
	_, _, version, err := client.ResolveCatalogAsset(context.Background(), w, 42, 7)
	if err != nil || version != "v2" {
		t.Fatalf("resolve: %s %v", version, err)
	}
	if _, _, _, err := client.ResolveCatalogAsset(context.Background(), w, 42, 999); err == nil {
		t.Fatal("foreign asset accepted")
	}
	path := filepath.Join(t.TempDir(), "artifact.zip")
	got, err := client.DownloadCatalogAsset(context.Background(), w.MetadataURL, 7, path, "sha256:"+digest)
	if err != nil || got != digest {
		t.Fatalf("download: %s %v", got, err)
	}
	badPath := filepath.Join(t.TempDir(), "bad.zip")
	if _, err := client.DownloadCatalogAsset(context.Background(), w.MetadataURL, 7, badPath, "sha256:wrong"); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Fatal("failed download left final artifact")
	}
}

func TestCatalogCandidateBlocksAutomaticActivation(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 42, "pending_version": "v2", "approved_version": "v2"}).Error; err != nil {
		t.Fatal(err)
	}
	allowed, err := r.releaseApproved("v2")
	if err != nil || allowed {
		t.Fatalf("catalog approved through normal poll: %v %v", allowed, err)
	}
	if _, err := r.state.SetDeploying("v3", "v1"); err == nil {
		t.Fatal("automatic deployment stole catalog candidate")
	}
	var count int64
	r.db.Model(&database.DeployLog{}).Count(&count)
	if count != 0 {
		t.Fatal("policy conflict created an attempt")
	}
}

func TestCatalogManifestVersionAndAssetBinding(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/releases/42":
			fmt.Fprint(w, `{"id":42,"tag_name":"release-tag","assets":[{"id":7,"name":"api.zip"},{"id":8,"name":"other.zip"}]}`)
		case "/repos/owner/repo/releases/tags/release-tag":
			fmt.Fprintf(w, `{"tag_name":"release-tag","assets":[{"id":9,"name":"version.json","browser_download_url":%q}]}`, server.URL+"/manifest")
		case "/manifest":
			fmt.Fprint(w, `{"services":{"api":{"version":"v2.7.1","artifact":"api.zip"}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewGitHubClient("", newTestLogger())
	client.apiBase = server.URL
	w := &database.Watcher{MetadataURL: "https://github.com/owner/repo/releases/latest/download/version.json", ServiceName: "api"}
	_, _, version, err := client.ResolveCatalogAsset(context.Background(), w, 42, 7)
	if err != nil || version != "v2.7.1" {
		t.Fatalf("manifest version: %s %v", version, err)
	}
	if _, _, _, err := client.ResolveCatalogAsset(context.Background(), w, 42, 8); err == nil {
		t.Fatal("another service's artifact was accepted")
	}
}

func TestCatalogDeployUsesStagedArtifactAndConsumesExactConfiguration(t *testing.T) {
	calls := installNSSMCommandMock(t)
	r, svc := candidateFixture(t)
	if err := r.db.AutoMigrate(&database.CatalogArtifact{}); err != nil {
		t.Fatal(err)
	}
	if err := r.db.Model(&svc).UpdateColumn("service_type", "static").Error; err != nil {
		t.Fatal(err)
	}
	var watcher database.Watcher
	r.db.First(&watcher, r.watcherID)
	watcher.MetadataURL, watcher.ServiceName, watcher.Paused = "https://github.com/owner/repo", "api", true
	r.db.Save(&watcher)
	now := time.Now().UTC()
	artifact := database.CatalogArtifact{WatcherID: watcher.ID, ServiceName: "api", Repository: "owner/repo", InstallDir: watcher.InstallDir, Tag: "v2", Version: "v2", AssetName: "api.zip", ArtifactURL: "https://github.com/owner/repo/releases/download/v2/api.zip", DownloadedAt: &now}
	r.db.Create(&artifact)
	path := CatalogArtifactPath(&artifact)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	createTestZipFile(t, path, map[string]string{"index.html": "chosen release", ".env": "VALUE=artifact"})
	data, _ := os.ReadFile(path)
	sum := sha256.Sum256(data)
	artifact.SHA256 = hex.EncodeToString(sum[:])
	r.db.Save(&artifact)
	r.db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2", "status": "approved"})
	r.db.Create(&database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: "v2", EnvContent: "VALUE=candidate"})
	if err := r.RunCatalog(WithTrace(context.Background(), NewPollTrace("", "manual")), artifact.ID, queueCatalogTestAttempt(t, r, "v2").ID); err != nil {
		t.Fatal(err)
	}
	r.db.First(&watcher, watcher.ID)
	r.db.First(&svc, svc.ID)
	if watcher.CurrentVersion != "v2" || watcher.PendingCatalogID != 0 || svc.EnvContent != "VALUE=candidate" || r.wcfg.Services[0].EnvContent != "VALUE=candidate" {
		t.Fatalf("wrong committed state: %s %d %s", watcher.CurrentVersion, watcher.PendingCatalogID, svc.EnvContent)
	}
	if err := ValidateCatalogArtifact(&watcher, &artifact); err != nil {
		t.Fatalf("staged ZIP not retained: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(watcher.InstallDir, "current", "index.html"))
	if err != nil || string(data) != "chosen release" {
		t.Fatalf("wrong artifact activated: %s %v", data, err)
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCatalogArtifact(&watcher, &artifact); err == nil {
		t.Fatal("modified staged artifact accepted")
	}
	r.db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2", "status": "approved"})
	queued := queueCatalogTestAttempt(t, r, "v2")
	beforeCalls := len(*calls)
	if err := r.RunCatalog(context.Background(), artifact.ID, queued.ID); err == nil {
		t.Fatal("modified artifact deployed")
	}
	r.db.First(&watcher, watcher.ID)
	if len(*calls) != beforeCalls || watcher.CurrentVersion != "v2" || watcher.ApprovedVersion != "" {
		t.Fatal("failed validation changed services or kept approval")
	}
	var attempt database.DeployLog
	r.db.Order("id desc").First(&attempt)
	if attempt.CompletedAt == nil || attempt.Status != "failed" {
		t.Fatal("failed validation left an open deployment")
	}

}

func TestCatalogPreflightFailureClosesQueuedAttempt(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprintf("missing_record=%t", missing), func(t *testing.T) {
			r, _ := candidateFixture(t)
			if err := r.db.AutoMigrate(&database.CatalogArtifact{}); err != nil {
				t.Fatal(err)
			}
			artifact := database.CatalogArtifact{WatcherID: r.watcherID, Version: "wrong-version"}
			if err := r.db.Create(&artifact).Error; err != nil {
				t.Fatal(err)
			}
			if missing {
				if err := r.db.Delete(&artifact).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{
				"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2", "status": "approved",
			}).Error; err != nil {
				t.Fatal(err)
			}
			attempt := database.DeployLog{WatcherID: r.watcherID, Version: "v2", Status: "in_progress", TriggeredBy: "manual", Kind: "deploy", Reason: "catalog_release"}
			if err := r.db.Create(&attempt).Error; err != nil {
				t.Fatal(err)
			}
			if err := r.RunCatalog(context.Background(), artifact.ID, attempt.ID); err == nil {
				t.Fatal("invalid catalog request succeeded")
			}
			var watcher database.Watcher
			if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
				t.Fatal(err)
			}
			if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
				t.Fatal(err)
			}
			if watcher.Status != "failed" || watcher.ApprovedVersion != "" || watcher.PendingCatalogID != artifact.ID || watcher.LastError == "" {
				t.Fatalf("preflight did not release approval and retain candidate: %+v", watcher)
			}
			if attempt.Status != "failed" || attempt.CompletedAt == nil {
				t.Fatalf("queued attempt remained open: %+v", attempt)
			}
		})
	}
}

func TestCatalogRejectionDoesNotFailAnotherCandidate(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{
		"pending_catalog_id": 20, "pending_version": "v3", "approved_version": "v3", "status": "approved",
	}).Error; err != nil {
		t.Fatal(err)
	}
	attempt := database.DeployLog{WatcherID: r.watcherID, Version: "v3", Status: "in_progress", Kind: "deploy", Reason: "catalog_release"}
	if err := r.db.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	a := NewAgent(r.db, r.global, r.log, nil, nil, nil, nil)
	a.rejectCatalogTrigger(CheckTrigger{WatcherID: r.watcherID, CatalogID: 10, DeployLogID: attempt.ID}, "stale trigger")
	var watcher database.Watcher
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.Status != "approved" || watcher.ApprovedVersion != "v3" || attempt.CompletedAt != nil {
		t.Fatal("stale rejection changed the active candidate")
	}
	a.recoverCatalogDeployments()
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.Status != "failed" || watcher.ApprovedVersion != "" || watcher.PendingCatalogID != 20 || attempt.CompletedAt == nil {
		t.Fatal("restart did not close the interrupted deployment and retain its candidate")
	}
}

func queueCatalogTestAttempt(t *testing.T, r *RepoWatcher, version string) database.DeployLog {
	t.Helper()
	now := time.Now().UTC()
	attempt := database.DeployLog{WatcherID: r.watcherID, Version: version, Status: "in_progress", TriggeredBy: "manual", Kind: "deploy", Reason: "catalog_release", StartedAt: &now}
	if err := r.db.Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	return attempt
}
