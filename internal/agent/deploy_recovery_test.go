package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeServiceManager struct {
	states      map[string]ServiceState
	stopErrors  map[string]error
	startErrors map[string]error
	startFails  map[string]int
	calls       []string
}

func (m *fakeServiceManager) Status(_ context.Context, name string) (ServiceState, error) {
	m.calls = append(m.calls, "status:"+name)
	state, ok := m.states[name]
	if !ok {
		return "", ErrServiceNotFound
	}
	return state, nil
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

	d := NewDeployer(&WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{
			{ServiceType: "nssm", WindowsServiceName: "api", BinaryName: "api.exe"},
		},
	}, "nssm.exe", newTestLogger(), nil)
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
