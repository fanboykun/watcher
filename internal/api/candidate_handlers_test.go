package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func candidateAPIFixture(t *testing.T) (*gin.Engine, *gorm.DB, database.Watcher, database.Service) {
	t.Helper()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	w := database.Watcher{Name: "test", ServiceName: "api", InstallDir: t.TempDir(), PendingVersion: "v2", Status: "pending_approval"}
	if err := db.Create(&w).Error; err != nil {
		t.Fatal(err)
	}
	svc := database.Service{WatcherID: w.ID, ServiceType: "nssm", WindowsServiceName: " api ", BinaryName: "api.exe", EnvFile: ".env", EnvContent: "VALUE=active"}
	if err := db.Create(&svc).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.AppConfig{WebAssetsPath: "/watcher"}
	router := NewRouter(db, "nssm", t.TempDir(), "test", "", ".env", cfg, agent.NewLoggerWithWriter("api", io.Discard, "error"), nil, make(chan agent.CheckTrigger, 1), make(chan struct{}, 1), nil, nil)
	return router, db, w, svc
}

func TestCandidateAPIEmptyEnvironmentAndAtomicBulkSave(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	path := fmt.Sprintf("/api/services/%d/revisions/v2", svc.ID)
	for _, body := range []string{`{"env_content":"first"}`, `{"env_content":""}`} {
		rec := authRequest(r, http.MethodPut, path, body, "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
		}
	}
	var revs []database.ServiceConfigRevision
	if err := db.Find(&revs).Error; err != nil {
		t.Fatal(err)
	}
	if len(revs) != 1 || revs[0].EnvContent != "" {
		t.Fatalf("empty env/upsert: %+v", revs)
	}
	body := fmt.Sprintf(`{"target_version":"v2","services":[{"service_id":%d,"env_content":"partial"},{"service_id":9999,"env_content":"invalid"}]}`, svc.ID)
	rec := authRequest(r, http.MethodPut, fmt.Sprintf("/api/watchers/%d/candidate", w.ID), body, "watcher")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid service: %d %s", rec.Code, rec.Body.String())
	}
	var got database.ServiceConfigRevision
	if err := db.First(&got, revs[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.EnvContent != "" {
		t.Fatal("failed bulk save partially persisted")
	}
}

func TestApprovalRejectsStaleVersionAndKeepsInterceptArmed(t *testing.T) {
	r, db, w, _ := candidateAPIFixture(t)
	if err := db.Model(&w).UpdateColumn("intercept_next_release", true).Error; err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/watchers/%d/approve", w.ID)
	rec := authRequest(r, http.MethodPost, path, `{"version":"v1"}`, "watcher")
	if rec.Code != http.StatusConflict {
		t.Fatalf("stale approval: %d %s", rec.Code, rec.Body.String())
	}
	rec = authRequest(r, http.MethodPost, path, `{"version":"v2"}`, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", rec.Code, rec.Body.String())
	}
	if err := db.First(&w, w.ID).Error; err != nil {
		t.Fatal(err)
	}
	if w.ApprovedVersion != "v2" || !w.InterceptNextRelease {
		t.Fatalf("wrong approval state: %+v", w)
	}
}

func TestWatcherCreatePersistsManualApprovalPolicy(t *testing.T) {
	r, db, _, _ := candidateAPIFixture(t)
	rec := authRequest(r, http.MethodPost, "/api/watchers", `{"name":"manual","service_name":"api","metadata_url":"https://example.com/version.json","install_dir":"test","auto_deploy":false}`, "watcher")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var w database.Watcher
	if err := db.Where("name = ?", "manual").First(&w).Error; err != nil {
		t.Fatal(err)
	}
	if w.AutoDeploy {
		t.Fatal("manual policy replaced by true default")
	}
}

func TestSnapshotInspectionIsHistoricalAndReadOnly(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	w.Services = []database.Service{svc}
	cfg := agent.WatcherConfigFromDB(&w)
	cfg.Services[0].EnvContent = "VALUE=historical"
	if err := agent.CaptureConfigSnapshot(cfg, "v1", agent.SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/services/%d/snapshots/v1/env", svc.ID)
	rec := authRequest(r, http.MethodGet, path, "", "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("read snapshot: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		EnvContent string `json:"env_content"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.EnvContent != "VALUE=historical" {
		t.Fatalf("snapshot=%q", body.EnvContent)
	}
	rec = authRequest(r, http.MethodPut, path, `{"env_content":"overwrite"}`, "watcher")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("snapshot write allowed: %d", rec.Code)
	}
	rec = authRequest(r, http.MethodPut, fmt.Sprintf("/api/watchers/%d/candidate", w.ID), fmt.Sprintf(`{"target_version":"v1","services":[{"service_id":%d,"env_content":"draft"}]}`, svc.ID), "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("candidate save: %d %s", rec.Code, rec.Body.String())
	}
	snapshot, err := agent.ReadServiceSnapshotEnv(cfg, "v1", 0)
	if err != nil || snapshot != "VALUE=historical" {
		t.Fatalf("draft changed history: %q %v", snapshot, err)
	}
	if err := db.Model(&svc).Update("env_file", "../outside.env").Error; err != nil {
		t.Fatal(err)
	}
	rec = authRequest(r, http.MethodGet, path, "", "watcher")
	if rec.Code == http.StatusOK {
		t.Fatal("unsafe snapshot env path accepted")
	}
}

func TestCandidateAndSnapshotSupportEncodedVersionTags(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	w.Services = []database.Service{svc}
	cfg := agent.WatcherConfigFromDB(&w)
	version := "release/2026#1"
	if err := agent.CaptureConfigSnapshot(cfg, version, agent.SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range []string{"/api", "/watcher/api"} {
		path := fmt.Sprintf("%s/services/%d/snapshots/release%%2F2026%%231/env", prefix, svc.ID)
		rec := authRequest(r, http.MethodGet, path, "", "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("encoded version read: %d %s", rec.Code, rec.Body.String())
		}
		path = fmt.Sprintf("%s/services/%d/revisions/release%%2F2026%%231", prefix, svc.ID)
		rec = authRequest(r, http.MethodPut, path, `{"env_content":"draft"}`, "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("encoded target save: %d %s", rec.Code, rec.Body.String())
		}
	}
	var rev database.ServiceConfigRevision
	if err := db.First(&rev).Error; err != nil {
		t.Fatal(err)
	}
	if rev.TargetVersion != version {
		t.Fatalf("target was not decoded correctly: %q", rev.TargetVersion)
	}
}

func TestCandidateManagedFilesRoundTripAndSaveIsolation(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	active := database.ServiceConfigFile{ServiceID: svc.ID, FilePath: "config/active.json", Target: "app_dir", Content: "active"}
	if err := db.Create(&active).Error; err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(w.InstallDir, "config", "active.json")
	if err := os.MkdirAll(filepath.Dir(live), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(live, []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	revisionPath := fmt.Sprintf("/api/services/%d/revisions/v2", svc.ID)
	body := `{"env_content":"candidate","config_files":[{"file_path":"config/new.json","target":"app_dir","content":"new"},{"file_path":"web.config","target":"release_dir","content":"release"}]}`
	rec := authRequest(r, http.MethodPut, revisionPath, body, "watcher")
	if rec.Code != http.StatusOK {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	var activeService database.Service
	if err := db.Preload("ConfigFiles").First(&activeService, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(live)
	if err != nil || string(data) != "active" || activeService.EnvContent != svc.EnvContent || len(activeService.ConfigFiles) != 1 || activeService.ConfigFiles[0].FilePath != active.FilePath {
		t.Fatalf("draft changed active configuration: %+v %s %v", activeService, data, err)
	}
	if _, err := os.Stat(filepath.Join(w.InstallDir, "config", "new.json")); !os.IsNotExist(err) {
		t.Fatalf("draft wrote live file: %v", err)
	}
	get := func() ServiceCandidateInfo {
		t.Helper()
		rec := authRequest(r, http.MethodGet, fmt.Sprintf("/api/watchers/%d/candidate?target=v2", w.ID), "", "watcher")
		if rec.Code != http.StatusOK {
			t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
		}
		var result WatcherCandidateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Services[0]
	}
	got := get()
	if len(got.ActiveConfigFiles) != 1 || len(got.CandidateConfigFiles) != 2 || !got.HasCandidateConfigFiles || !got.IsModified {
		t.Fatalf("round trip: %+v", got)
	}
	// Existing env-only clients preserve a previously prepared file override.
	for _, body := range []string{`{"env_content":"changed"}`, `{"env_content":"changed","config_files":null}`} {
		rec = authRequest(r, http.MethodPut, revisionPath, body, "watcher")
		if rec.Code != http.StatusOK || len(get().CandidateConfigFiles) != 2 {
			t.Fatalf("legacy save lost file override: %d %s", rec.Code, rec.Body.String())
		}
	}
	// Bulk saves support explicit removal and keep it across an omitted update.
	for _, body := range []string{
		fmt.Sprintf(`{"target_version":"v2","services":[{"service_id":%d,"env_content":"changed","config_files":[]}]}`, svc.ID),
		fmt.Sprintf(`{"target_version":"v2","services":[{"service_id":%d,"env_content":"changed"}]}`, svc.ID),
	} {
		rec = authRequest(r, http.MethodPut, fmt.Sprintf("/api/watchers/%d/candidate", w.ID), body, "watcher")
		got = get()
		if rec.Code != http.StatusOK || !got.HasCandidateConfigFiles || got.CandidateConfigFiles == nil || len(got.CandidateConfigFiles) != 0 {
			t.Fatalf("explicit removal lost: %d %s %+v", rec.Code, rec.Body.String(), got)
		}
	}
}

func TestCandidateManagedFilesRejectUnsafeAndConflictingTargets(t *testing.T) {
	r, db, w, svc := candidateAPIFixture(t)
	for _, file := range []string{
		`{"file_path":"../outside","target":"app_dir"}`,
		`{"file_path":"..\\outside","target":"release_dir"}`,
		`{"file_path":"C:\\outside","target":"app_dir"}`,
		`{"file_path":"/outside","target":"app_dir"}`,
		`{"file_path":".watcher/snapshots/fake","target":"app_dir"}`,
		`{"file_path":"current/config.json","target":"app_dir"}`,
		`{"file_path":"config.json","target":"unknown"}`,
		`{"file_path":".env","target":"app_dir"}`,
		`{"file_path":"x.json","target":"app_dir"},{"file_path":"X.json","target":"app_dir"}`,
		`{"file_path":"settings","target":"app_dir"},{"file_path":"settings/app.json","target":"app_dir"}`,
		`{"file_path":".env/app.json","target":"app_dir"}`,
	} {
		body := `{"env_content":"draft","config_files":[` + file + `]}`
		rec := authRequest(r, http.MethodPut, fmt.Sprintf("/api/services/%d/revisions/next", svc.ID), body, "watcher")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("accepted unsafe files %s: %d %s", file, rec.Code, rec.Body.String())
		}
		body = fmt.Sprintf(`{"target_version":"next","services":[{"service_id":%d,"env_content":"draft","config_files":[%s]}]}`, svc.ID, file)
		rec = authRequest(r, http.MethodPut, fmt.Sprintf("/api/watchers/%d/candidate", w.ID), body, "watcher")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("bulk accepted unsafe files %s: %d %s", file, rec.Code, rec.Body.String())
		}
	}
	var count int64
	if err := db.Model(&database.ServiceConfigRevision{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("invalid revisions persisted: %d %v", count, err)
	}
}
