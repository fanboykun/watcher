package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type lifecycleAPIFakeManager struct {
	startErr   error
	stopErr    error
	restartErr error
	calls      []string
}

func (m *lifecycleAPIFakeManager) Status(context.Context, string) (agent.ServiceState, error) {
	return agent.ServiceStateRunning, nil
}

func (m *lifecycleAPIFakeManager) Start(_ context.Context, name string) error {
	m.calls = append(m.calls, "start:"+name)
	return m.startErr
}

func (m *lifecycleAPIFakeManager) Stop(_ context.Context, name string) error {
	m.calls = append(m.calls, "stop:"+name)
	return m.stopErr
}

func (m *lifecycleAPIFakeManager) Restart(_ context.Context, name string) error {
	m.calls = append(m.calls, "restart:"+name)
	return m.restartErr
}

func newServiceLifecycleAPIFixture(t *testing.T, serviceType string, manager agent.ServiceManager, windows bool) (*gin.Engine, uint) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&database.Watcher{}, &database.Service{}, &database.ServiceConfigFile{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	watcher := database.Watcher{Name: "alpha", ServiceName: "api", InstallDir: t.TempDir()}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatalf("create watcher: %v", err)
	}
	service := database.Service{WatcherID: watcher.ID, ServiceType: serviceType, WindowsServiceName: "alpha-api", BinaryName: "api.exe"}
	if err := db.Create(&service).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}

	h := &Handler{db: db, serviceManager: manager, isWindows: func() bool { return windows }}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/services/:id/start", h.StartService)
	router.POST("/services/:id/stop", h.StopService)
	router.POST("/services/:id/restart", h.RestartService)
	return router, service.ID
}

func TestServiceLifecycleAPIIntegration(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		configure  func(*lifecycleAPIFakeManager)
		wantStatus int
		wantCall   string
		wantBody   string
	}{
		{name: "start success", operation: "start", wantStatus: http.StatusOK, wantCall: "start:alpha-api", wantBody: string(agent.ServiceStateRunning)},
		{name: "stop success", operation: "stop", wantStatus: http.StatusOK, wantCall: "stop:alpha-api", wantBody: string(agent.ServiceStateStopped)},
		{name: "restart success", operation: "restart", wantStatus: http.StatusOK, wantCall: "restart:alpha-api", wantBody: string(agent.ServiceStateRunning)},
		{
			name:      "start failure preserves NSSM detail",
			operation: "start",
			configure: func(manager *lifecycleAPIFakeManager) {
				manager.startErr = errors.New("nssm start alpha-api: exit status 5 (output: Access is denied)")
			},
			wantStatus: http.StatusInternalServerError,
			wantCall:   "start:alpha-api",
			wantBody:   "Access is denied",
		},
		{
			name:      "stop timeout preserves last state",
			operation: "stop",
			configure: func(manager *lifecycleAPIFakeManager) {
				manager.stopErr = errors.New("waiting for service alpha-api to reach SERVICE_STOPPED (last state: SERVICE_STOP_PENDING): context deadline exceeded")
			},
			wantStatus: http.StatusInternalServerError,
			wantCall:   "stop:alpha-api",
			wantBody:   "SERVICE_STOP_PENDING",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &lifecycleAPIFakeManager{}
			if tt.configure != nil {
				tt.configure(manager)
			}
			router, serviceID := newServiceLifecycleAPIFixture(t, "nssm", manager, true)
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/services/%d/%s", serviceID, tt.operation), nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body)
			}
			if got := strings.Join(manager.calls, ","); got != tt.wantCall {
				t.Fatalf("manager calls = %q, want %q", got, tt.wantCall)
			}
			if !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want text %q", response.Body, tt.wantBody)
			}
		})
	}
}

func TestServiceLifecycleAPIRejectsUnsupportedTargets(t *testing.T) {
	t.Run("non NSSM service", func(t *testing.T) {
		manager := &lifecycleAPIFakeManager{}
		router, serviceID := newServiceLifecycleAPIFixture(t, "iis", manager, true)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/services/%d/start", serviceID), nil))
		if response.Code != http.StatusBadRequest || len(manager.calls) != 0 {
			t.Fatalf("status=%d calls=%v, want 400 and no manager call", response.Code, manager.calls)
		}
	})

	t.Run("non Windows runtime", func(t *testing.T) {
		manager := &lifecycleAPIFakeManager{}
		router, serviceID := newServiceLifecycleAPIFixture(t, "nssm", manager, false)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, fmt.Sprintf("/services/%d/start", serviceID), nil))
		if response.Code != http.StatusServiceUnavailable || len(manager.calls) != 0 {
			t.Fatalf("status=%d calls=%v, want 503 and no manager call", response.Code, manager.calls)
		}
	})
}

func TestNSSMServiceCleanupIntegration(t *testing.T) {
	tests := []struct {
		name          string
		stopErr       error
		removeOutput  string
		removeErr     error
		wantErr       string
		wantRemoveRun bool
	}{
		{name: "stop then remove", wantRemoveRun: true},
		{name: "already missing still removes idempotently", stopErr: agent.ErrServiceNotFound, removeOutput: "SERVICE_DOES_NOT_EXIST", removeErr: errors.New("exit status 3"), wantRemoveRun: true},
		{name: "stop failure prevents remove", stopErr: errors.New("Access is denied"), wantErr: "failed to stop service", wantRemoveRun: false},
		{name: "remove failure is returned", removeOutput: "Access is denied", removeErr: errors.New("exit status 5"), wantErr: "failed to remove service", wantRemoveRun: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &lifecycleAPIFakeManager{stopErr: tt.stopErr}
			removeCalled := false
			h := &Handler{
				serviceManager: manager,
				isWindows:      func() bool { return true },
				runNSSMCommand: func(_ context.Context, args ...string) ([]byte, error) {
					removeCalled = true
					if got := strings.Join(args, " "); got != "remove alpha-api confirm" {
						t.Fatalf("remove args = %q", got)
					}
					return []byte(tt.removeOutput), tt.removeErr
				},
			}
			svc := &database.Service{ServiceType: "nssm", WindowsServiceName: "alpha-api"}
			err := h.cleanupServiceRuntime(context.Background(), svc)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("cleanupServiceRuntime returned error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("cleanupServiceRuntime error = %v, want text %q", err, tt.wantErr)
			}
			if removeCalled != tt.wantRemoveRun {
				t.Fatalf("remove called = %v, want %v", removeCalled, tt.wantRemoveRun)
			}
		})
	}
}
