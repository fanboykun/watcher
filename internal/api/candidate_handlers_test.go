package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
