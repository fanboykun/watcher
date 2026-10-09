package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

type statusTestManager struct {
	lifecycleAPIFakeManager
	state agent.ServiceState
	err   error
	probe func(context.Context)
	calls int
}

func (m *statusTestManager) Status(ctx context.Context, _ string) (agent.ServiceState, error) {
	m.calls++
	if m.probe != nil {
		m.probe(ctx)
	}
	return m.state, m.err
}

func TestServiceStatus(t *testing.T) {
	tests := []struct {
		name, serviceType, wantRuntime, wantHealth string
		state                                      agent.ServiceState
		err                                        error
		windows, fallback, noURL, badURL           bool
		httpStatus                                 int
	}{
		{name: "running and healthy", state: agent.ServiceStateRunning, windows: true, httpStatus: 200, wantRuntime: "running", wantHealth: "healthy"},
		{name: "stopped still probes health", state: agent.ServiceStateStopped, windows: true, httpStatus: 200, wantRuntime: "stopped", wantHealth: "healthy"},
		{name: "running and unhealthy", state: agent.ServiceStateRunning, windows: true, httpStatus: 503, wantRuntime: "running", wantHealth: "unhealthy"},
		{name: "health request error", state: agent.ServiceStateRunning, windows: true, badURL: true, wantRuntime: "running", wantHealth: "error"},
		{name: "pending", state: agent.ServiceStateStartPending, windows: true, httpStatus: 200, wantRuntime: "start_pending", wantHealth: "healthy"},
		{name: "not installed", err: fmt.Errorf("missing: %w", agent.ErrServiceNotFound), windows: true, httpStatus: 200, wantRuntime: "not_installed", wantHealth: "healthy"},
		{name: "NSSM unavailable", err: errors.New("NSSM executable missing"), windows: true, httpStatus: 200, wantRuntime: "unknown", wantHealth: "healthy"},
		{name: "non Windows", httpStatus: 200, wantRuntime: "unknown", wantHealth: "healthy"},
		{name: "IIS", serviceType: "iis", windows: true, httpStatus: 200, wantRuntime: "not_applicable", wantHealth: "healthy"},
		{name: "watcher health fallback", state: agent.ServiceStateRunning, windows: true, fallback: true, httpStatus: 200, wantRuntime: "running", wantHealth: "healthy"},
		{name: "no health URL clears stale health", state: agent.ServiceStateStopped, windows: true, noURL: true, wantRuntime: "stopped", wantHealth: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			healthRequested := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				healthRequested <- struct{}{}
				w.WriteHeader(tt.httpStatus)
			}))
			defer server.Close()
			db, err := database.NewDB(filepath.Join(t.TempDir(), "status.db"))
			if err != nil {
				t.Fatal(err)
			}
			watcher := database.Watcher{Name: "test", ServiceName: "test", InstallDir: t.TempDir()}
			if tt.fallback {
				watcher.HcURL = server.URL
			}
			if err := db.Create(&watcher).Error; err != nil {
				t.Fatal(err)
			}
			serviceType := tt.serviceType
			if serviceType == "" {
				serviceType = "nssm"
			}
			svc := database.Service{WatcherID: watcher.ID, ServiceType: serviceType, WindowsServiceName: "test", LastHealthStatus: "unhealthy", LastHealthHTTPStatus: 500, LastHealthError: "old error"}
			if !tt.noURL && !tt.fallback {
				svc.HealthCheckURL = server.URL
			}
			if err := db.Create(&svc).Error; err != nil {
				t.Fatal(err)
			}
			if tt.badURL {
				if err := db.Model(&svc).UpdateColumn("health_check_url", ":bad").Error; err != nil {
					t.Fatal(err)
				}
			}
			manager := &statusTestManager{state: tt.state, err: tt.err}
			// Runtime waits for the health request, proving probes are concurrent.
			if !tt.noURL && !tt.badURL {
				manager.probe = func(ctx context.Context) {
					select {
					case <-healthRequested:
					case <-ctx.Done():
						t.Error("health request did not start while NSSM status was pending")
					}
				}
			}
			h := &Handler{db: db, serviceManager: manager, isWindows: func() bool { return tt.windows }}
			router := gin.New()
			router.GET("/services/:id/status", h.GetServiceStatus)
			router.GET("/services", h.ListAllServices)
			router.GET("/watchers/:id/services", h.ListServices)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/services/%d/status", svc.ID), nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
			}
			var got database.Service
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.LastServiceStatus != tt.wantRuntime || got.LastHealthStatus != tt.wantHealth {
				t.Fatalf("runtime=%s health=%s", got.LastServiceStatus, got.LastHealthStatus)
			}
			if got.LastServiceCheckedAt == nil || got.LastHealthCheckedAt == nil {
				t.Fatal("missing observation timestamps")
			}
			if tt.badURL && got.LastHealthError == "" {
				t.Fatal("missing health request error")
			}
			if !tt.badURL && got.LastHealthError != "" {
				t.Fatalf("stale health error: %s", got.LastHealthError)
			}
			if got.LastHealthHTTPStatus != tt.httpStatus {
				t.Fatalf("HTTP status=%d", got.LastHealthHTTPStatus)
			}
			wantCalls := 0
			if tt.windows && serviceType == "nssm" {
				wantCalls = 1
			}
			if manager.calls != wantCalls {
				t.Fatalf("NSSM calls=%d want=%d", manager.calls, wantCalls)
			}
			var saved database.Service
			if err := db.First(&saved, svc.ID).Error; err != nil {
				t.Fatal(err)
			}
			if saved.LastServiceStatus != got.LastServiceStatus || saved.LastHealthStatus != got.LastHealthStatus {
				t.Fatal("observations not persisted")
			}
			if !saved.UpdatedAt.Equal(svc.UpdatedAt) {
				t.Fatal("status probe changed configuration updated_at")
			}
			var event database.HealthEvent
			if err := db.First(&event).Error; err != nil {
				t.Fatal(err)
			}
			if event.Status != tt.wantHealth || event.PreviousStatus != "unhealthy" {
				t.Fatalf("health event=%+v", event)
			}
			for _, path := range []string{"/services", fmt.Sprintf("/watchers/%d/services", watcher.ID)} {
				list := httptest.NewRecorder()
				router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, path, nil))
				var services []database.Service
				if err := json.Unmarshal(list.Body.Bytes(), &services); err != nil {
					t.Fatal(err)
				}
				if len(services) != 1 || services[0].LastServiceStatus != tt.wantRuntime || services[0].LastHealthStatus != tt.wantHealth {
					t.Fatalf("list %s=%s", path, list.Body)
				}
			}
		})
	}
}

func TestServiceStatusRejectsInvalidIDsAndRollsBackFailedWrites(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "status.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "test", ServiceName: "test", InstallDir: t.TempDir()}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	svc := database.Service{WatcherID: watcher.ID, WindowsServiceName: "test", LastServiceStatus: "stopped"}
	if err := db.Create(&svc).Error; err != nil {
		t.Fatal(err)
	}
	manager := &statusTestManager{state: agent.ServiceStateRunning}
	h := &Handler{db: db, serviceManager: manager, isWindows: func() bool { return true }}
	router := gin.New()
	router.GET("/services/:id/status", h.GetServiceStatus)
	for _, tt := range []struct {
		id     string
		status int
	}{{"invalid", 400}, {"999999", 404}} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/services/"+tt.id+"/status", nil))
		if rec.Code != tt.status {
			t.Fatalf("id=%s status=%d", tt.id, rec.Code)
		}
	}
	if manager.calls != 0 {
		t.Fatal("NSSM queried for an invalid ID")
	}
	// Fail the service update after the health event insert to verify atomicity.
	if err := db.Exec("CREATE TRIGGER reject_status BEFORE UPDATE ON services BEGIN SELECT RAISE(ABORT, 'test write failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/services/%d/status", svc.ID), nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var saved database.Service
	if err := db.First(&saved, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.LastServiceStatus != "stopped" || saved.LastServiceCheckedAt != nil {
		t.Fatal("failed write changed stored status")
	}
	var count int64
	if err := db.Model(&database.HealthEvent{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed status update left a health event")
	}
}
