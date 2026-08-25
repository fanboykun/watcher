package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeServiceManager struct {
	states       map[string]ServiceState
	statusErrors map[string]error
	stopErrors   map[string]error
	startErrors  map[string]error
	startFails   map[string]int
	calls        []string
}

func (m *fakeServiceManager) Status(_ context.Context, name string) (ServiceState, error) {
	m.calls = append(m.calls, "status:"+name)
	if err := m.statusErrors[name]; err != nil {
		return "", err
	}
	state, ok := m.states[name]
	if !ok {
		return "", ErrServiceNotFound
	}
	return state, nil
}

func TestServiceExistsUsesSharedNSSMStatus(t *testing.T) {
	tests := []struct {
		name       string
		manager    *fakeServiceManager
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "registered service",
			manager:    &fakeServiceManager{states: map[string]ServiceState{"api": ServiceStateRunning}},
			wantExists: true,
		},
		{
			name:    "missing service",
			manager: &fakeServiceManager{states: map[string]ServiceState{}},
		},
		{
			name:    "status query failure",
			manager: &fakeServiceManager{states: map[string]ServiceState{}, statusErrors: map[string]error{"api": errors.New("SCM unavailable")}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewDeployer(&WatcherConfig{}, "nssm.exe", newTestLogger(), nil)
			d.serviceManager = tt.manager

			exists, err := d.serviceExists(context.Background(), "api")
			if (err != nil) != tt.wantErr {
				t.Fatalf("serviceExists error = %v, wantErr %v", err, tt.wantErr)
			}
			if exists != tt.wantExists {
				t.Fatalf("serviceExists = %v, want %v", exists, tt.wantExists)
			}
			if got := strings.Join(tt.manager.calls, ","); got != "status:api" {
				t.Fatalf("manager calls = %q, want status:api", got)
			}
		})
	}
}

func TestEnsureServiceNSSMRegistrationMatrix(t *testing.T) {
	t.Run("missing service installs and tolerates optional setting failure", func(t *testing.T) {
		manager := &fakeServiceManager{states: map[string]ServiceState{}}
		var calls []string
		originalRunCommand := runCommand
		t.Cleanup(func() { runCommand = originalRunCommand })
		runCommand = func(_ string, args ...string) ([]byte, error) {
			calls = append(calls, strings.Join(args, " "))
			if len(args) >= 4 && args[0] == "set" && args[2] == "AppRotateOnline" {
				return []byte("rotation unsupported"), errors.New("exit status 1")
			}
			return []byte("ok"), nil
		}

		d := NewDeployer(&WatcherConfig{InstallDir: `D:\apps\api`}, "nssm.exe", newTestLogger(), nil)
		d.serviceManager = manager
		err := d.ensureService(context.Background(), ServiceConfig{
			WindowsServiceName: "api",
			StartArguments:     "--port 8080",
			EnvFile:            ".env",
		}, `D:\apps\api\current\api.exe`)
		if err != nil {
			t.Fatalf("ensureService returned error: %v", err)
		}
		joined := strings.Join(calls, "\n")
		for _, want := range []string{
			`install api D:\apps\api\current\api.exe`,
			`set api AppDirectory D:\apps\api`,
			"set api AppParameters --port 8080",
			"set api Start SERVICE_AUTO_START",
			"set api AppEnvironmentExtra ENV_FILE=.env",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("NSSM calls missing %q:\n%s", want, joined)
			}
		}
	})

	t.Run("required setting failure aborts", func(t *testing.T) {
		manager := &fakeServiceManager{states: map[string]ServiceState{"api": ServiceStateStopped}}
		originalRunCommand := runCommand
		t.Cleanup(func() { runCommand = originalRunCommand })
		runCommand = func(_ string, args ...string) ([]byte, error) {
			if len(args) >= 4 && args[0] == "set" && args[2] == "Application" {
				return []byte("Access is denied"), errors.New("exit status 5")
			}
			return []byte("ok"), nil
		}

		d := NewDeployer(&WatcherConfig{InstallDir: `D:\apps\api`}, "nssm.exe", newTestLogger(), nil)
		d.serviceManager = manager
		err := d.ensureService(context.Background(), ServiceConfig{WindowsServiceName: "api"}, `D:\apps\api\current\api.exe`)
		if err == nil || !strings.Contains(err.Error(), "nssm set api Application") || !strings.Contains(err.Error(), "Access is denied") {
			t.Fatalf("ensureService error = %v, want required setting failure", err)
		}
	})

	t.Run("install failure aborts before settings", func(t *testing.T) {
		manager := &fakeServiceManager{states: map[string]ServiceState{}}
		var calls []string
		originalRunCommand := runCommand
		t.Cleanup(func() { runCommand = originalRunCommand })
		runCommand = func(_ string, args ...string) ([]byte, error) {
			calls = append(calls, strings.Join(args, " "))
			return []byte("Access is denied"), errors.New("exit status 5")
		}

		d := NewDeployer(&WatcherConfig{}, "nssm.exe", newTestLogger(), nil)
		d.serviceManager = manager
		err := d.ensureService(context.Background(), ServiceConfig{WindowsServiceName: "api"}, `D:\api.exe`)
		if err == nil || !strings.Contains(err.Error(), "nssm install api") {
			t.Fatalf("ensureService error = %v, want install failure", err)
		}
		if len(calls) != 1 || !strings.HasPrefix(calls[0], "install api ") {
			t.Fatalf("commands after install failure = %#v, want install only", calls)
		}
	})
}

func (m *fakeServiceManager) Stop(_ context.Context, name string) error {
	m.calls = append(m.calls, "stop:"+name)
	if err := m.stopErrors[name]; err != nil {
		return err
	}
	m.states[name] = ServiceStateStopped
	return nil
}

func (m *fakeServiceManager) Start(_ context.Context, name string) error {
	m.calls = append(m.calls, "start:"+name)
	if err := m.startErrors[name]; err != nil && (m.startFails == nil || m.startFails[name] != 0) {
		if m.startFails != nil && m.startFails[name] > 0 {
			m.startFails[name]--
		}
		return err
	}
	m.states[name] = ServiceStateRunning
	return nil
}

func (m *fakeServiceManager) Restart(ctx context.Context, name string) error {
	if err := m.Stop(ctx, name); err != nil {
		return err
	}
	return m.Start(ctx, name)
}

func TestStopServicesRecoversEarlierServicesAfterPartialFailure(t *testing.T) {
	manager := &fakeServiceManager{
		states: map[string]ServiceState{
			"api-a": ServiceStateRunning,
			"api-b": ServiceStateRunning,
		},
		stopErrors:  map[string]error{"api-b": errors.New("SCM rejected stop")},
		startErrors: map[string]error{},
	}
	d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
		{ServiceType: "nssm", WindowsServiceName: "api-a"},
		{ServiceType: "nssm", WindowsServiceName: "api-b"},
	}}, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager

	_, err := d.stopServices(context.Background(), "deploy")
	if err == nil || !strings.Contains(err.Error(), "stop api-b during deploy") {
		t.Fatalf("stopServices error = %v, want api-b stop failure", err)
	}
	if got := manager.states["api-a"]; got != ServiceStateRunning {
		t.Fatalf("api-a state = %s, want recovered SERVICE_RUNNING", got)
	}
	if got, want := strings.Join(manager.calls, ","), "status:api-a,stop:api-a,status:api-b,stop:api-b,start:api-a"; got != want {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

func TestPrepareReleaseRejectsMissingBinaryBeforeServiceLifecycle(t *testing.T) {
	stagedDir := t.TempDir()
	d := NewDeployer(&WatcherConfig{Services: []ServiceConfig{
		{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
	}}, "nssm.exe", newTestLogger(), nil)

	err := d.prepareRelease(stagedDir)
	if err == nil || !strings.Contains(err.Error(), "api.exe") {
		t.Fatalf("prepareRelease error = %v, want missing binary error", err)
	}
}

func TestWriteManagedFileRejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	err := writeManagedFile(root, filepath.Join("..", "outside.env"), "secret")
	if err == nil || !strings.Contains(err.Error(), "escapes root") {
		t.Fatalf("writeManagedFile error = %v, want root escape rejection", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(root), "outside.env")); !os.IsNotExist(statErr) {
		t.Fatalf("outside file was created, stat err=%v", statErr)
	}
}

func TestSwapCurrentFallbackPreservesOldCurrentWhenCandidateFails(t *testing.T) {
	root := t.TempDir()
	currentDir := filepath.Join(root, "current")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "old.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	originalRunCommand := runCommand
	t.Cleanup(func() { runCommand = originalRunCommand })
	runCommand = func(string, ...string) ([]byte, error) {
		return []byte("mklink failed"), errors.New("exit status 1")
	}

	d := NewDeployer(&WatcherConfig{}, "nssm.exe", newTestLogger(), nil)
	err := d.swapCurrent(filepath.Join(root, "missing-release"), currentDir)
	if err == nil {
		t.Fatal("expected candidate preparation failure")
	}
	content, readErr := os.ReadFile(filepath.Join(currentDir, "old.txt"))
	if readErr != nil || string(content) != "old" {
		t.Fatalf("old current was not preserved: content=%q err=%v", content, readErr)
	}
}

func TestSwapCurrentFallbackCommitsPreparedCandidate(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "release")
	currentDir := filepath.Join(root, "current")
	if err := os.MkdirAll(releaseDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(releaseDir, "new.txt"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(currentDir, "old.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	originalRunCommand := runCommand
	t.Cleanup(func() { runCommand = originalRunCommand })
	runCommand = func(string, ...string) ([]byte, error) {
		return []byte("mklink unavailable"), errors.New("exit status 1")
	}

	d := NewDeployer(&WatcherConfig{}, "nssm.exe", newTestLogger(), nil)
	if err := d.swapCurrent(releaseDir, currentDir); err != nil {
		t.Fatalf("swapCurrent returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(currentDir, "new.txt"))
	if err != nil || string(content) != "new" {
		t.Fatalf("new current content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(currentDir, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old current content still exists, stat err=%v", err)
	}
	matches, err := filepath.Glob(currentDir + ".previous-*")
	if err != nil || len(matches) != 0 {
		t.Fatalf("previous current paths were not cleaned up: %v (err=%v)", matches, err)
	}
}

func TestReleasePromotionCanRestoreExistingRelease(t *testing.T) {
	root := t.TempDir()
	releasesDir := filepath.Join(root, "releases")
	stagedDir := filepath.Join(releasesDir, "staged")
	releaseDir := filepath.Join(releasesDir, "v1")
	for path, content := range map[string]string{stagedDir: "new", releaseDir: "old"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "app.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	d := NewDeployer(&WatcherConfig{InstallDir: root}, "nssm.exe", newTestLogger(), nil)
	promotion, err := d.promoteRelease(stagedDir, releaseDir)
	if err != nil {
		t.Fatalf("promoteRelease returned error: %v", err)
	}
	if err := promotion.restore(); err != nil {
		t.Fatalf("restore returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(releaseDir, "app.exe"))
	if err != nil || string(content) != "old" {
		t.Fatalf("restored release content=%q err=%v", content, err)
	}
}

func TestActivatedDeployBackupRecoveryIsNotVersionRollback(t *testing.T) {
	root := t.TempDir()
	releaseDir := filepath.Join(root, "releases", "v1")
	backupDir := filepath.Join(root, ".watcher-release-backup")
	currentDir := filepath.Join(root, "current")
	for path, content := range map[string]string{
		releaseDir: "new",
		backupDir:  "previous-attempt",
		currentDir: "new",
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "api.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	originalRunCommand := runCommand
	t.Cleanup(func() { runCommand = originalRunCommand })
	runCommand = func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "/C" {
			return []byte("mklink unavailable"), errors.New("exit status 1")
		}
		return []byte("ok"), nil
	}

	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}
	captureConfigSnapshotsForTest(t, wcfg, "v1", "v2")
	d := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager
	cause := errors.New("start service api during deploy: failed")

	err := d.recoverActivatedDeployment(context.Background(), "v1", currentDir, "", cause, &releasePromotion{
		releaseDir: releaseDir,
		backupDir:  backupDir,
	})
	if err == nil || !strings.Contains(err.Error(), "restored release backup for v1") {
		t.Fatalf("recovery error = %v, want release-backup recovery result", err)
	}
	_, _, rollbackTo, rollbackFailed := classifyDeployFailure(err)
	if rollbackTo != "" || rollbackFailed {
		t.Fatalf("backup recovery classified as rollback: rollbackTo=%q rollbackFailed=%v", rollbackTo, rollbackFailed)
	}
	content, readErr := os.ReadFile(filepath.Join(currentDir, "api.exe"))
	if readErr != nil || string(content) != "previous-attempt" {
		t.Fatalf("current release content=%q err=%v, want restored attempt backup", content, readErr)
	}
}

func TestManualRollbackRestoresOriginalReleaseWhenTargetStartFails(t *testing.T) {
	root := t.TempDir()
	releasesDir := filepath.Join(root, "releases")
	oldRelease := filepath.Join(releasesDir, "v1")
	targetRelease := filepath.Join(releasesDir, "v2")
	for path, content := range map[string]string{oldRelease: "old", targetRelease: "target"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "api.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(oldRelease, filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{"api": errors.New("target failed to start")},
		startFails:  map[string]int{"api": 1},
	}
	originalRunCommand := runCommand
	t.Cleanup(func() { runCommand = originalRunCommand })
	runCommand = func(_ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "/C" {
			return []byte("mklink unavailable"), errors.New("exit status 1")
		}
		if len(args) > 0 && args[0] == "status" {
			return []byte(ServiceStateRunning), nil
		}
		return []byte("ok"), nil
	}

	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}
	captureConfigSnapshotsForTest(t, wcfg, "v1", "v2")
	d := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager

	err := d.Rollback(context.Background(), "v2")
	if err == nil || !strings.Contains(err.Error(), "rollback to v2 failed, restored v1") {
		t.Fatalf("Rollback error = %v, want restored-original result", err)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "current", "api.exe"))
	if readErr != nil || string(content) != "old" {
		t.Fatalf("current release content=%q err=%v, want old release", content, readErr)
	}
	if got := manager.states["api"]; got != ServiceStateRunning {
		t.Fatalf("api state = %s, want SERVICE_RUNNING", got)
	}
}

func TestManualRollbackRejectsMissingTargetBeforeServiceLifecycle(t *testing.T) {
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	d := NewDeployer(&WatcherConfig{
		InstallDir: t.TempDir(),
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager

	err := d.Rollback(context.Background(), "missing")
	if err == nil || !strings.Contains(err.Error(), "rollback target") {
		t.Fatalf("Rollback error = %v, want missing target", err)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("service lifecycle calls = %v, want none before target validation", manager.calls)
	}
}

func TestManualRollbackReportsTargetAndOriginalStartFailures(t *testing.T) {
	root := t.TempDir()
	previousDir := filepath.Join(root, "releases", "v1")
	targetDir := filepath.Join(root, "releases", "v2")
	for path, content := range map[string]string{previousDir: "previous", targetDir: "target"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "api.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(previousDir, filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}
	installNSSMCommandMock(t)
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{"api": errors.New("service start failed")},
		startFails:  map[string]int{"api": 2},
	}
	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}
	captureConfigSnapshotsForTest(t, wcfg, "v1", "v2")
	d := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager

	err := d.Rollback(context.Background(), "v2")
	if err == nil || !strings.Contains(err.Error(), "service start failed") || !strings.Contains(err.Error(), "restart original release v1") {
		t.Fatalf("Rollback error = %v, want target and original start failures", err)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "current", "api.exe"))
	if readErr != nil || string(content) != "previous" {
		t.Fatalf("current content=%q err=%v, want original release reactivated even though start failed", content, readErr)
	}
}

func TestManualRollbackHealthFailureRestoresOriginalRelease(t *testing.T) {
	root := t.TempDir()
	previousDir := filepath.Join(root, "releases", "v1")
	targetDir := filepath.Join(root, "releases", "v2")
	for path, content := range map[string]string{previousDir: "previous", targetDir: "target"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "api.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(previousDir, filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)
	installNSSMCommandMock(t)
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	wcfg := &WatcherConfig{
		InstallDir: root,
		HealthCheck: HealthCheckConfig{
			Enabled: true, URL: healthServer.URL, Retries: 1, TimeoutSec: 1,
		},
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}
	captureConfigSnapshotsForTest(t, wcfg, "v1", "v2")
	d := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	d.serviceManager = manager

	err := d.Rollback(context.Background(), "v2")
	if err == nil || !strings.Contains(err.Error(), "rollback to v2 failed, restored v1") || !strings.Contains(err.Error(), "health check failed") {
		t.Fatalf("Rollback error = %v, want failed target health and restored original", err)
	}
	content, readErr := os.ReadFile(filepath.Join(root, "current", "api.exe"))
	if readErr != nil || string(content) != "previous" {
		t.Fatalf("current content=%q err=%v, want previous", content, readErr)
	}
}
