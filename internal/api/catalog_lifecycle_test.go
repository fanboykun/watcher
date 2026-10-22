package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func TestDiscardCatalogPreservesRollbackHighWatermark(t *testing.T) {
	for _, tc := range []struct{ pin, discard, want string }{
		{"v5.0.0", "v2.0.0", "v5.0.0"},
		{"v2.0.0", "v5.0.0", "v5.0.0"},
		{"", "older-tag", "older-tag"},
		{"rollback-tag", "older-tag", "rollback-tag"},
	} {
		t.Run(tc.pin+"_"+tc.discard, func(t *testing.T) {
			router, db, watcher, _ := candidateAPIFixture(t)
			if err := db.Model(&watcher).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": tc.discard, "max_ignored_version": tc.pin, "status": "failed"}).Error; err != nil {
				t.Fatal(err)
			}
			response := authRequest(router, http.MethodPost, fmt.Sprintf("/api/watchers/%d/discard", watcher.ID), `{}`, "watcher")
			if response.Code != http.StatusOK {
				t.Fatalf("discard: %d %s", response.Code, response.Body.String())
			}
			if err := db.First(&watcher, watcher.ID).Error; err != nil {
				t.Fatal(err)
			}
			if watcher.MaxIgnoredVersion != tc.want {
				t.Fatalf("pin=%s want=%s", watcher.MaxIgnoredVersion, tc.want)
			}
		})
	}
}

func TestCatalogAPIThroughAgentCachedActivationAndHealthRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local simulation uses a temporary appcmd shim on PATH")
	}
	commands := t.TempDir()
	if err := os.WriteFile(filepath.Join(commands, `C:\Windows\System32\inetsrv\appcmd.exe`), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", commands+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, scenario := range []struct {
		name       string
		failHealth bool
		retry      bool
	}{
		{name: "successful activation"},
		{name: "health rollback and discard", failHealth: true},
		{name: "health rollback and explicit retry", failHealth: true, retry: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			db, err := database.NewDB(filepath.Join(root, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer sqlDB.Close()
			var rejectSelected atomic.Bool
			rejectSelected.Store(scenario.failHealth)
			health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				data, err := os.ReadFile(filepath.Join(root, "current", "api.exe"))
				if err != nil || (rejectSelected.Load() && string(data) == "selected") {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer health.Close()
			watcher := database.Watcher{Name: "catalog", ServiceName: "api", MetadataURL: "https://github.com/owner/repo", ReleaseRef: "fixed-poll-reference", InstallDir: root, Paused: true, CurrentVersion: "v1", Status: "healthy", HcEnabled: true, HcURL: health.URL, HcRetries: 1, HcTimeoutSec: 1}
			if err := db.Create(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			svc := database.Service{WatcherID: watcher.ID, ServiceType: "static", WindowsServiceName: "static", EnvFile: ".env", EnvContent: "VALUE=active"}
			if err := db.Create(&svc).Error; err != nil {
				t.Fatal(err)
			}
			watcher.Services = []database.Service{svc}
			if err := os.MkdirAll(filepath.Join(root, "releases", "v1"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "current"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{filepath.Join(root, "releases", "v1", "api.exe"), filepath.Join(root, "current", "api.exe")} {
				if err := os.WriteFile(path, []byte("previous"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".env"), []byte(svc.EnvContent), 0600); err != nil {
				t.Fatal(err)
			}
			if err := agent.CaptureConfigSnapshot(agent.WatcherConfigFromDB(&watcher), "v1", agent.SnapshotSourceDeployment); err != nil {
				t.Fatal(err)
			}
			log := agent.NewLoggerWithWriter("catalog-test", io.Discard, "error")
			cfg := &config.AppConfig{NssmPath: "unused-nssm"}
			queue := make(chan agent.CheckTrigger, 10)
			bus := agent.NewWatcherEventBus()
			events, unsubscribe := bus.Subscribe(watcher.ID)
			defer unsubscribe()
			runtime := agent.NewAgent(db, cfg, log, bus, queue, nil, nil)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); runtime.Run(ctx) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("agent did not join")
				}
			}()
			select {
			case <-runtime.Ready():
			case <-time.After(5 * time.Second):
				t.Fatal("agent not ready")
			}
			h := NewHandler(db, "unused-nssm", root, "test", "", ".env", cfg, log, bus, queue, nil, nil, nil)
			fake := &fakeCatalog{}
			h.catalogFactory = func(string, *agent.Logger) catalogClient { return fake }
			gin.SetMode(gin.ReleaseMode)
			router := gin.New()
			registerAPIRoutes(router.Group("/api"), h)
			base := fmt.Sprintf("/api/watchers/%d", watcher.ID)
			response := authRequest(router, http.MethodPost, base+"/releases/download", `{"release_id":11,"asset_id":22}`, "watcher")
			if response.Code != http.StatusCreated {
				t.Fatalf("download: %d %s", response.Code, response.Body.String())
			}
			var artifact database.CatalogArtifact
			if err := json.Unmarshal(response.Body.Bytes(), &artifact); err != nil {
				t.Fatal(err)
			}
			response = authRequest(router, http.MethodPost, fmt.Sprintf("%s/releases/%d/candidate", base, artifact.ID), `{}`, "watcher")
			if response.Code != http.StatusOK {
				t.Fatalf("select: %d %s", response.Code, response.Body.String())
			}
			body := fmt.Sprintf(`{"target_version":"v2","services":[{"service_id":%d,"env_content":"VALUE=exact"}]}`, svc.ID)
			response = authRequest(router, http.MethodPut, base+"/candidate", body, "watcher")
			if response.Code != http.StatusOK {
				t.Fatalf("configure: %d %s", response.Code, response.Body.String())
			}
			if err := db.Create(&database.ServiceConfigRevision{ServiceID: svc.ID, TargetVersion: "next", EnvContent: "VALUE=next"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&svc, svc.ID).Error; err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil || string(before) != "VALUE=active" || svc.EnvContent != "VALUE=active" {
				t.Fatal("candidate preparation changed active configuration")
			}
			before, err = os.ReadFile(filepath.Join(root, "current", "api.exe"))
			if err != nil || string(before) != "previous" {
				t.Fatal("candidate preparation changed active release files")
			}
			// Deployment must use the saved ZIP even with remote asset downloads disabled.
			fake.failDownload = true
			deploy := func(path, body string) database.DeployLog {
				t.Helper()
				response := authRequest(router, http.MethodPost, path, body, "watcher")
				if response.Code != http.StatusAccepted {
					t.Fatalf("deploy: %d %s", response.Code, response.Body.String())
				}
				var accepted struct {
					DeployLogID uint   `json:"deploy_log_id"`
					PollID      string `json:"poll_id"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
					t.Fatal(err)
				}
				if accepted.DeployLogID == 0 || accepted.PollID == "" || response.Header().Get("X-Poll-ID") != accepted.PollID {
					t.Fatalf("missing exact deployment identity: %s", response.Body.String())
				}
				deadline := time.NewTimer(10 * time.Second)
				defer deadline.Stop()
				for {
					select {
					case event := <-events:
						if event.Type == "poll_completed" && event.Data["poll_id"] == accepted.PollID {
							var attempt database.DeployLog
							if err := db.First(&attempt, accepted.DeployLogID).Error; err != nil {
								t.Fatal(err)
							}
							if attempt.CompletedAt == nil || attempt.Reason != "catalog_release" || attempt.Version != "v2" || attempt.TriggeredBy != "manual" {
								t.Fatalf("unexpected completed attempt: %+v", attempt)
							}
							return attempt
						}
					case <-deadline.C:
						t.Fatal("catalog cycle did not complete")
					}
				}
			}
			attempt := deploy(base+"/approve", `{"version":"v2"}`)
			if err := db.First(&watcher, watcher.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&svc, svc.ID).Error; err != nil {
				t.Fatal(err)
			}
			active, err := os.ReadFile(filepath.Join(root, "current", "api.exe"))
			if err != nil {
				t.Fatal(err)
			}
			env, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if watcher.ReleaseRef != "fixed-poll-reference" || watcher.Status != "healthy" || !watcher.Paused || attempt.CompletedAt == nil {
				t.Fatalf("unexpected completed state: watcher=%+v attempt=%+v", watcher, attempt)
			}
			if scenario.failHealth {
				if watcher.CurrentVersion != "v1" || watcher.PendingCatalogID != artifact.ID || watcher.ApprovedVersion != "" || attempt.Status != "failed" || attempt.FailurePhase != "health_check" || string(active) != "previous" || string(env) != "VALUE=active" || svc.EnvContent != "VALUE=active" {
					t.Fatalf("health rollback did not restore previous release and snapshot: watcher=%+v attempt=%+v active=%q env=%q", watcher, attempt, active, env)
				}
				var rollback database.DeployLog
				if err := db.Where("parent_attempt_id = ? AND kind = ?", attempt.ID, "rollback").First(&rollback).Error; err != nil || rollback.Status != "succeeded" || rollback.CompletedAt == nil {
					t.Fatalf("rollback lineage: %+v %v", rollback, err)
				}
				var drafts []database.ServiceConfigRevision
				if err := db.Where("service_id = ?", svc.ID).Order("target_version").Find(&drafts).Error; err != nil || len(drafts) != 2 || drafts[0].TargetVersion != "next" || drafts[1].TargetVersion != "v2" || drafts[1].EnvContent != "VALUE=exact" {
					t.Fatalf("rollback lost retained configuration: %+v %v", drafts, err)
				}
				if scenario.retry {
					rejectSelected.Store(false)
					retry := deploy(fmt.Sprintf("%s/releases/%d/deploy", base, artifact.ID), `{}`)
					if retry.Status != "succeeded" || retry.ID == attempt.ID || retry.ID == rollback.ID {
						t.Fatalf("retry did not create a successful distinct attempt: %+v", retry)
					}
					if err := db.First(&watcher, watcher.ID).Error; err != nil {
						t.Fatal(err)
					}
					if err := db.First(&svc, svc.ID).Error; err != nil {
						t.Fatal(err)
					}
					active, err = os.ReadFile(filepath.Join(root, "current", "api.exe"))
					if err != nil {
						t.Fatal(err)
					}
					env, err = os.ReadFile(filepath.Join(root, ".env"))
					if err != nil {
						t.Fatal(err)
					}
					if watcher.CurrentVersion != "v2" || watcher.Status != "healthy" || watcher.PendingCatalogID != 0 || watcher.PendingVersion != "" || watcher.ApprovedVersion != "" || !watcher.Paused || watcher.ReleaseRef != "fixed-poll-reference" || string(active) != "selected" || string(env) != "VALUE=exact" || svc.EnvContent != "VALUE=exact" {
						t.Fatalf("retry did not activate retained candidate: watcher=%+v active=%q env=%q", watcher, active, env)
					}
				} else {
					response = authRequest(router, http.MethodPost, base+"/discard", `{}`, "watcher")
					if response.Code != http.StatusOK {
						t.Fatalf("discard recovered candidate: %d %s", response.Code, response.Body.String())
					}
				}
			} else if watcher.CurrentVersion != "v2" || watcher.PendingCatalogID != 0 || attempt.Status != "succeeded" || string(active) != "selected" || string(env) != "VALUE=exact" || svc.EnvContent != "VALUE=exact" {
				t.Fatalf("wrong cached activation: watcher=%+v attempt=%+v active=%q env=%q", watcher, attempt, active, env)
			}
			var revisions []database.ServiceConfigRevision
			if err := db.Where("service_id = ?", svc.ID).Find(&revisions).Error; err != nil {
				t.Fatal(err)
			}
			if len(revisions) != 1 || revisions[0].TargetVersion != "next" {
				t.Fatalf("unused draft consumed: %+v", revisions)
			}
			if err := db.First(&artifact, artifact.ID).Error; err != nil {
				t.Fatal(err)
			}
			if err := agent.ValidateCatalogArtifact(&watcher, &artifact); err != nil {
				t.Fatalf("cached ZIP was not retained: %v", err)
			}
		})
	}
}
