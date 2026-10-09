package api

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

type fakeCatalog struct{ failDownload bool }

func (f *fakeCatalog) ListReleaseCatalog(ctx context.Context, url string, page int) (*agent.ReleaseCatalog, error) {
	return &agent.ReleaseCatalog{Page: page, Releases: []agent.CatalogRelease{{ID: 11, TagName: "v2", Assets: []agent.CatalogAsset{{ID: 22, Name: "api.zip"}}}}}, nil
}
func (f *fakeCatalog) ResolveCatalogAsset(ctx context.Context, w *database.Watcher, releaseID, assetID int64) (*agent.CatalogRelease, *agent.CatalogAsset, string, error) {
	if releaseID != 11 || assetID != 22 {
		return nil, nil, "", fmt.Errorf("invalid selection")
	}
	return &agent.CatalogRelease{ID: 11, TagName: "v2"}, &agent.CatalogAsset{ID: 22, Name: "api.zip", BrowserDownloadURL: "https://github.com/owner/repo/releases/download/v2/api.zip"}, "v2", nil
}
func (f *fakeCatalog) DownloadCatalogAsset(ctx context.Context, repo string, id int64, path, digest string) (string, error) {
	if f.failDownload {
		return "", fmt.Errorf("download failed")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	file, err := os.Create(path)
	if err != nil {
		return "", err
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("api.exe")
	_, _ = entry.Write([]byte("selected"))
	_ = archive.Close()
	_ = file.Close()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func TestCatalogAPISelectionOwnershipAndExactQueue(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", InstallDir: t.TempDir(), GitHubToken: "watcher-token", Paused: true}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	queue := make(chan agent.CheckTrigger, 1)
	log := agent.NewLoggerWithWriter("api", io.Discard, "error")
	h := NewHandler(db, "nssm", t.TempDir(), "test", "global-token", ".env", &config.AppConfig{}, log, nil, queue, nil, nil, nil)
	fake := &fakeCatalog{}
	h.catalogFactory = func(token string, log *agent.Logger) catalogClient {
		if token != "watcher-token" {
			t.Error("global token replaced watcher override")
		}
		return fake
	}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	registerAPIRoutes(router.Group("/api"), h)
	base := fmt.Sprintf("/api/watchers/%d", watcher.ID)
	rec := authRequest(router, http.MethodGet, base+"/releases?page=2", "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	rec = authRequest(router, http.MethodPost, base+"/releases/download", `{"release_id":11,"asset_id":22}`, "watcher")
	if rec.Code != http.StatusCreated {
		t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
	}
	var artifact database.CatalogArtifact
	if err := json.Unmarshal(rec.Body.Bytes(), &artifact); err != nil {
		t.Fatal(err)
	}
	var stored database.CatalogArtifact
	db.First(&stored, artifact.ID)
	relative, err := filepath.Rel(watcher.InstallDir, agent.CatalogArtifactPath(&stored))
	if err != nil || !strings.HasPrefix(relative, "downloads"+string(filepath.Separator)) {
		t.Fatal("artifact was not stored in watcher downloads")
	}
	rec = authRequest(router, http.MethodGet, base+"/versions", "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("downloaded versions: %d %s", rec.Code, rec.Body.String())
	}
	var versionList struct {
		Downloads []database.CatalogArtifact `json:"downloads"`
		Versions  []agent.ReleaseInfo        `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &versionList); err != nil {
		t.Fatal(err)
	}
	if len(versionList.Downloads) != 1 || versionList.Downloads[0].Version != "v2" || len(versionList.Versions) != 0 {
		t.Fatal("download is missing as a deployable target or was exposed as a rollback target")
	}
	path := fmt.Sprintf("%s/releases/%d", base, artifact.ID)
	rec = authRequest(router, http.MethodPost, path+"/candidate", `{}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("candidate: %d %s", rec.Code, rec.Body.String())
	}
	db.First(&watcher, watcher.ID)
	if watcher.PendingCatalogID != artifact.ID || watcher.PendingVersion != "v2" || watcher.CurrentVersion != "" {
		t.Fatal("candidate modified active state or bound wrong version")
	}
	for _, action := range []string{"redeploy", "rollback"} {
		rec = authRequest(router, http.MethodPost, base+"/"+action, `{"version":"v1"}`, "watcher")
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s interfered with selected catalog candidate: %d %s", action, rec.Code, rec.Body.String())
		}
	}
	other := database.Watcher{Name: "other", InstallDir: t.TempDir()}
	db.Create(&other)
	rec = authRequest(router, http.MethodPost, fmt.Sprintf("/api/watchers/%d/releases/%d/deploy", other.ID, artifact.ID), `{}`, "watcher")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross watcher artifact accepted: %d", rec.Code)
	}
	rec = authRequest(router, http.MethodPost, base+"/approve", `{"version":"v9"}`, "watcher")
	if rec.Code != http.StatusConflict {
		t.Fatal("stale approval accepted")
	}
	queue <- agent.CheckTrigger{WatcherID: other.ID}
	rec = authRequest(router, http.MethodPost, path+"/deploy", `{}`, "watcher")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("full queue: %d %s", rec.Code, rec.Body.String())
	}
	var count int64
	db.Model(&database.DeployLog{}).Count(&count)
	db.First(&watcher, watcher.ID)
	if count != 0 || watcher.Status != "pending_approval" || watcher.ApprovedVersion != "" {
		t.Fatal("full queue left reserved deployment")
	}
	<-queue
	rec = authRequest(router, http.MethodPost, base+"/approve", `{"version":"v2"}`, "watcher")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("approve selected: %d %s", rec.Code, rec.Body.String())
	}
	trigger := <-queue
	if trigger.CatalogID != artifact.ID || trigger.WatcherID != watcher.ID || trigger.Trace.RequestID == "" || trigger.Trace.PollID == "" {
		t.Fatalf("not an exact correlated deployment: %+v", trigger)
	}
	rec = authRequest(router, http.MethodPost, path+"/deploy", `{}`, "watcher")
	if rec.Code != http.StatusConflict {
		t.Fatal("duplicate deployment accepted")
	}
	fake.failDownload = true
	rec = authRequest(router, http.MethodPost, base+"/releases/download", `{"release_id":11,"asset_id":22}`, "watcher")
	if rec.Code != http.StatusBadGateway {
		t.Fatal("failed download reported success")
	}
	db.Model(&database.CatalogArtifact{}).Count(&count)
	if count != 1 {
		t.Fatalf("failed download retained record: %d", count)
	}
}
