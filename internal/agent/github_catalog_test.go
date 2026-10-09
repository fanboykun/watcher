package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
)

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
	r.db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2"})
	r.db.Create(&database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: "v2", EnvContent: "VALUE=candidate"})
	if err := r.RunCatalog(WithTrace(context.Background(), NewPollTrace("", "manual")), artifact.ID); err != nil {
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
	r.db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": artifact.ID, "pending_version": "v2", "approved_version": "v2"})
	r.db.Create(&database.DeployLog{WatcherID: watcher.ID, Version: "v2", Status: "in_progress", TriggeredBy: "manual", Kind: "deploy", Reason: "catalog_release"})
	beforeCalls := len(*calls)
	if err := r.RunCatalog(context.Background(), artifact.ID); err == nil {
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
