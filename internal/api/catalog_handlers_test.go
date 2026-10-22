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
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type fakeCatalog struct {
	failDownload bool
	failList     bool
	download     func(context.Context, string) (string, error)
}

func (f *fakeCatalog) ListReleaseCatalog(ctx context.Context, url string, page int) (*agent.ReleaseCatalog, error) {
	if f.failList {
		return nil, fmt.Errorf("GitHub unavailable")
	}
	return &agent.ReleaseCatalog{Page: page, Releases: []agent.CatalogRelease{{ID: 11, TagName: "v2", Assets: []agent.CatalogAsset{{ID: 22, Name: "api.zip"}}}}}, nil
}
func (f *fakeCatalog) ResolveCatalogAsset(ctx context.Context, w *database.Watcher, releaseID, assetID int64) (*agent.CatalogRelease, *agent.CatalogAsset, string, error) {
	if releaseID != 11 || assetID != 22 {
		return nil, nil, "", fmt.Errorf("invalid selection")
	}
	return &agent.CatalogRelease{ID: 11, TagName: "v2"}, &agent.CatalogAsset{ID: 22, Name: "api.zip", BrowserDownloadURL: "https://github.com/owner/repo/releases/download/v2/api.zip"}, "v2", nil
}
func (f *fakeCatalog) DownloadCatalogAsset(ctx context.Context, repo string, id int64, path, digest string) (string, error) {
	if f.download != nil {
		return f.download(ctx, path)
	}
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

func TestCatalogDownloadFailureCleansLocalTarget(t *testing.T) {
	for _, scenario := range []string{"failed download", "partial download", "cancelled successful client", "ready persistence failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", InstallDir: t.TempDir()}
			if err := db.Create(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fake := &fakeCatalog{}
			switch scenario {
			case "failed download":
				fake.failDownload = true
			case "partial download", "cancelled successful client":
				fake.download = func(ctx context.Context, path string) (string, error) {
					if scenario == "cancelled successful client" {
						sum, err := (&fakeCatalog{}).DownloadCatalogAsset(ctx, watcher.MetadataURL, 22, path, "")
						cancel()
						return sum, err
					}
					if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
						return "", err
					}
					if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
						return "", err
					}
					return "", fmt.Errorf("connection interrupted")
				}
			case "ready persistence failure":
				if err := db.Callback().Update().Before("gorm:update").Register("test:catalog_ready_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "catalog_artifacts" {
						tx.AddError(fmt.Errorf("ready persistence failed"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			log := agent.NewLoggerWithWriter("api", io.Discard, "error")
			h := NewHandler(db, "nssm", t.TempDir(), "test", "", ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
			h.catalogFactory = func(string, *agent.Logger) catalogClient { return fake }
			router := gin.New()
			registerAPIRoutes(router.Group("/api"), h)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/watchers/%d/releases/download", watcher.ID), strings.NewReader(`{"release_id":11,"asset_id":22}`)).WithContext(ctx)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer watcher")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadGateway {
				t.Fatalf("failed download returned %d: %s", rec.Code, rec.Body.String())
			}
			var rows int64
			if err := db.Model(&database.CatalogArtifact{}).Count(&rows).Error; err != nil || rows != 0 {
				t.Fatalf("failed download left catalog rows: %d %v", rows, err)
			}
			files, err := filepath.Glob(filepath.Join(watcher.InstallDir, "downloads", "catalog", "*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("failed download left local targets: %v %v", files, err)
			}
		})
	}
}

func TestCatalogTokenPrecedenceAndPublicAccess(t *testing.T) {
	for _, tc := range []struct{ name, watcherToken, globalToken, want string }{
		{name: "watcher override", watcherToken: " watcher-token ", globalToken: "global-token", want: "watcher-token"},
		{name: "global fallback", globalToken: "global-token", want: "global-token"},
		{name: "blank watcher fallback", watcherToken: " ", globalToken: "global-token", want: "global-token"},
		{name: "public without token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", InstallDir: t.TempDir(), GitHubToken: tc.watcherToken}
			if err := db.Create(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			log := agent.NewLoggerWithWriter("api", io.Discard, "error")
			h := NewHandler(db, "nssm", t.TempDir(), "test", tc.globalToken, ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
			calls := 0
			h.catalogFactory = func(token string, _ *agent.Logger) catalogClient {
				calls++
				if token != tc.want {
					t.Error("incorrect catalog token precedence")
				}
				return &fakeCatalog{}
			}
			router := gin.New()
			registerAPIRoutes(router.Group("/api"), h)
			base := fmt.Sprintf("/api/watchers/%d/releases", watcher.ID)
			if rec := authRequest(router, http.MethodGet, base, "", "watcher"); rec.Code != http.StatusOK {
				t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
			}
			if rec := authRequest(router, http.MethodPost, base+"/download", `{"release_id":11,"asset_id":22}`, "watcher"); rec.Code != http.StatusCreated {
				t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
			}
			if calls != 2 {
				t.Fatalf("factory calls=%d, want 2", calls)
			}
		})
	}
}

func TestCatalogDownloadedVersionsSurviveReopenAndGitHubFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", InstallDir: t.TempDir()}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	log := agent.NewLoggerWithWriter("api", io.Discard, "error")
	h := NewHandler(db, "nssm", t.TempDir(), "test", "", ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
	h.catalogFactory = func(string, *agent.Logger) catalogClient { return &fakeCatalog{} }
	router := gin.New()
	registerAPIRoutes(router.Group("/api"), h)
	base := fmt.Sprintf("/api/watchers/%d", watcher.ID)
	var ids []uint
	for range 2 {
		rec := authRequest(router, http.MethodPost, base+"/releases/download", `{"release_id":11,"asset_id":22}`, "watcher")
		if rec.Code != http.StatusCreated {
			t.Fatalf("download: %d %s", rec.Code, rec.Body.String())
		}
		var artifact database.CatalogArtifact
		if err := json.Unmarshal(rec.Body.Bytes(), &artifact); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, artifact.ID)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	h = NewHandler(db, "nssm", t.TempDir(), "test", "", ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
	remoteCalls := 0
	h.catalogFactory = func(string, *agent.Logger) catalogClient {
		remoteCalls++
		return &fakeCatalog{failList: true}
	}
	router = gin.New()
	registerAPIRoutes(router.Group("/api"), h)
	if rec := authRequest(router, http.MethodGet, base+"/releases", "", "watcher"); rec.Code != http.StatusBadGateway {
		t.Fatalf("expected remote failure: %d %s", rec.Code, rec.Body.String())
	}
	rec := authRequest(router, http.MethodGet, base+"/versions", "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("local versions: %d %s", rec.Code, rec.Body.String())
	}
	var result struct {
		Downloads []database.CatalogArtifact `json:"downloads"`
		Versions  []agent.ReleaseInfo        `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if remoteCalls != 1 || len(result.Downloads) != 2 || len(result.Versions) != 0 {
		t.Fatalf("local targets depend on remote listing or became rollback versions: %+v remote calls=%d", result, remoteCalls)
	}
	for i, artifact := range result.Downloads {
		if artifact.ID != ids[1-i] || artifact.Tag != "v2" || artifact.Version != "v2" || artifact.AssetName != "api.zip" {
			t.Fatalf("local target identity changed: %+v", artifact)
		}
		// Server-only storage identity is intentionally absent from the API response.
		if err := db.First(&artifact, artifact.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := agent.ValidateCatalogArtifact(&watcher, &artifact); err != nil {
			t.Fatalf("reopened target is not deployable: %v", err)
		}
	}
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
	if trigger.DeployLogID == 0 || trigger.CatalogID != artifact.ID || trigger.WatcherID != watcher.ID || trigger.Trace.RequestID == "" || trigger.Trace.PollID == "" {
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

func TestDiscardRetainedCatalogCandidate(t *testing.T) {
	for _, tc := range []struct {
		name, status, approved string
		catalog, active        bool
		want                   int
	}{
		{name: "failed download can be discarded", status: "failed", catalog: true, want: http.StatusOK},
		{name: "automatic rollback can be discarded", status: "healthy", catalog: true, want: http.StatusOK},
		{name: "pending candidate can be discarded", status: "pending_approval", catalog: true, want: http.StatusOK},
		{name: "approved deployment is protected", status: "approved", approved: "v2", catalog: true, want: http.StatusBadRequest},
		{name: "running deployment is protected", status: "deploying", approved: "v2", catalog: true, want: http.StatusBadRequest},
		{name: "open attempt is protected", status: "failed", catalog: true, active: true, want: http.StatusConflict},
		{name: "ordinary failure is not a candidate", status: "failed", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			watcher := database.Watcher{Name: "catalog", ServiceName: "api", InstallDir: t.TempDir(), CurrentVersion: "v1", PendingVersion: "v2", ApprovedVersion: tc.approved, Status: tc.status}
			if err := db.Create(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			artifact := database.CatalogArtifact{WatcherID: watcher.ID, Version: "v2"}
			if err := db.Create(&artifact).Error; err != nil {
				t.Fatal(err)
			}
			if tc.catalog {
				if err := db.Model(&watcher).UpdateColumn("pending_catalog_id", artifact.ID).Error; err != nil {
					t.Fatal(err)
				}
			}
			svc := database.Service{WatcherID: watcher.ID, EnvContent: "ACTIVE=1"}
			if err := db.Create(&svc).Error; err != nil {
				t.Fatal(err)
			}
			for _, version := range []string{"v2", "next"} {
				if err := db.Create(&database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: version, EnvContent: "STAGED=1"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if tc.active {
				if err := db.Create(&database.DeployLog{WatcherID: watcher.ID, Version: "v2", Status: "in_progress"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			log := agent.NewLoggerWithWriter("api", io.Discard, "error")
			h := NewHandler(db, "nssm", t.TempDir(), "test", "", ".env", &config.AppConfig{}, log, nil, nil, nil, nil, nil)
			router := gin.New()
			registerAPIRoutes(router.Group("/api"), h)
			rec := authRequest(router, http.MethodPost, fmt.Sprintf("/api/watchers/%d/discard", watcher.ID), `{}`, "watcher")
			if rec.Code != tc.want {
				t.Fatalf("discard: %d %s", rec.Code, rec.Body.String())
			}
			var stored database.Watcher
			if err := db.First(&stored, watcher.ID).Error; err != nil {
				t.Fatal(err)
			}
			var revisions int64
			db.Model(&database.ServiceConfigRevision{}).Where("service_id = ?", svc.ID).Count(&revisions)
			if tc.want == http.StatusOK {
				if stored.PendingCatalogID != 0 || stored.PendingVersion != "" || stored.ApprovedVersion != "" || stored.CurrentVersion != "v1" || revisions != 1 {
					t.Fatalf("discard did not release exact candidate: %+v, revisions=%d", stored, revisions)
				}
				if err := db.First(&artifact, artifact.ID).Error; err != nil {
					t.Fatalf("discard removed downloaded target: %v", err)
				}
			} else if stored.PendingCatalogID != watcher.PendingCatalogID || stored.PendingVersion != "v2" || revisions != 2 {
				t.Fatal("rejected discard changed candidate state")
			}
		})
	}
}
