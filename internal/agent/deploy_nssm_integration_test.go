package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func installNSSMCommandMock(t *testing.T) *[]string {
	t.Helper()
	var calls []string
	originalRunCommand := runCommand
	t.Cleanup(func() { runCommand = originalRunCommand })
	runCommand = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "cmd" && len(args) > 0 && args[0] == "/C" {
			return []byte("mklink unavailable in test"), errors.New("exit status 1")
		}
		return []byte("ok"), nil
	}
	return &calls
}

func newNSSMDeployFixture(t *testing.T, content string) (*Deployer, string, string) {
	t.Helper()
	root := t.TempDir()
	zipPath := filepath.Join(root, "artifact.zip")
	createTestZipFile(t, zipPath, map[string]string{"api.exe": content})
	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}
	return NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil), root, zipPath
}

func TestDeployIntegrationFirstInstallNSSMPendingThenRunning(t *testing.T) {
	calls := installNSSMCommandMock(t)
	deployer, root, zipPath := newNSSMDeployFixture(t, "target")
	runner := newScriptedNSSMRunner(t,
		nssmCommandStep{args: "status api", output: "The specified service does not exist as an installed service.", err: errors.New("exit status 3")},
		nssmCommandStep{args: "status api", output: "The specified service does not exist as an installed service.", err: errors.New("exit status 3")},
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{
			args:   "start api",
			output: "api: Unexpected status SERVICE_START_PENDING in response to START control.",
			err:    errors.New("exit status 1"),
		},
		stateStep("api", ServiceStateStartPending),
		stateStep("api", ServiceStateRunning),
	)
	deployer.serviceManager = newTestServiceManager(runner.run)

	if err := deployer.Deploy(context.Background(), "v1", zipPath, ""); err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	runner.assertDone()
	if content, err := os.ReadFile(filepath.Join(root, "current", "api.exe")); err != nil || string(content) != "target" {
		t.Fatalf("current content=%q err=%v, want target", content, err)
	}
	joined := strings.Join(*calls, "\n")
	if !strings.Contains(joined, "nssm.exe install api ") {
		t.Fatalf("NSSM install was not called:\n%s", joined)
	}
}

func TestDeployIntegrationFirstInstallStartFailureHasNoRollback(t *testing.T) {
	installNSSMCommandMock(t)
	deployer, _, zipPath := newNSSMDeployFixture(t, "target")
	runner := newScriptedNSSMRunner(t,
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		nssmCommandStep{args: "status api", output: "SERVICE_DOES_NOT_EXIST", err: errors.New("exit status 3")},
		stateStep("api", ServiceStateStopped),
		nssmCommandStep{args: "start api", output: "The service cannot be started", err: errors.New("exit status 1")},
		stateStep("api", ServiceStateStopped),
	)
	deployer.serviceManager = newTestServiceManager(runner.run)

	err := deployer.Deploy(context.Background(), "v1", zipPath, "")
	if err == nil {
		t.Fatal("Deploy returned nil; want start failure")
	}
	var recoveryErr *deployRecoveryError
	if !errors.As(err, &recoveryErr) {
		t.Fatalf("Deploy error type = %T, want deployRecoveryError", err)
	}
	if recoveryErr.rollbackVersion != "" || !recoveryErr.noRollbackVersion {
		t.Fatalf("recovery = %#v, want no rollback version", recoveryErr)
	}
	if !strings.Contains(err.Error(), "no previous version to roll back to") {
		t.Fatalf("Deploy error = %v, want no-previous-version detail", err)
	}
	runner.assertDone()
}

func TestDeployIntegrationStartFailureRollsBackRecordedPreviousVersion(t *testing.T) {
	installNSSMCommandMock(t)
	deployer, root, zipPath := newNSSMDeployFixture(t, "target")
	previousDir := filepath.Join(root, "releases", "v1")
	if err := os.MkdirAll(previousDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previousDir, "api.exe"), []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "current"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "current", "api.exe"), []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	captureConfigSnapshotsForTest(t, deployer.wcfg, "v1")
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{"api": errors.New("target failed to start")},
		startFails:  map[string]int{"api": 1},
	}
	deployer.serviceManager = manager

	err := deployer.Deploy(context.Background(), "v2", zipPath, "v1")
	if err == nil {
		t.Fatal("Deploy returned nil; want target start failure with rollback")
	}
	var recoveryErr *deployRecoveryError
	if !errors.As(err, &recoveryErr) || recoveryErr.rollbackVersion != "v1" || recoveryErr.rollbackErr != nil {
		t.Fatalf("recovery error = %#v, want successful rollback to v1", recoveryErr)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, "current", "api.exe")); readErr != nil || string(content) != "previous" {
		t.Fatalf("current content=%q err=%v, want previous", content, readErr)
	}
	if manager.states["api"] != ServiceStateRunning {
		t.Fatalf("service state=%s, want SERVICE_RUNNING", manager.states["api"])
	}
}

func TestDeployIntegrationHealthFailureRollsBackRecordedPreviousVersion(t *testing.T) {
	installNSSMCommandMock(t)
	deployer, root, zipPath := newNSSMDeployFixture(t, "target")
	previousDir := filepath.Join(root, "releases", "v1")
	if err := os.MkdirAll(previousDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(previousDir, "api.exe"), []byte("previous"), 0644); err != nil {
		t.Fatal(err)
	}
	captureConfigSnapshotsForTest(t, deployer.wcfg, "v1")
	var healthCalls atomic.Int32
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if healthCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)
	deployer.wcfg.HealthCheck = HealthCheckConfig{Enabled: true, URL: healthServer.URL, Retries: 1, TimeoutSec: 1}
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	deployer.serviceManager = manager

	err := deployer.Deploy(context.Background(), "v2", zipPath, "v1")
	var recoveryErr *deployRecoveryError
	if err == nil || !errors.As(err, &recoveryErr) || recoveryErr.rollbackVersion != "v1" {
		t.Fatalf("Deploy error = %v, want health failure rolled back to v1", err)
	}
	if !strings.Contains(recoveryErr.cause.Error(), "health check failed") {
		t.Fatalf("recovery cause = %v, want health-check failure", recoveryErr.cause)
	}
}
