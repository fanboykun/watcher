package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManualRollbackRestoresTargetVersionEnvironment(t *testing.T) {
	root := t.TempDir()
	oldRelease := filepath.Join(root, "releases", "v1")
	latestRelease := filepath.Join(root, "releases", "v2")
	for path, content := range map[string]string{oldRelease: "old", latestRelease: "latest"} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "api.exe"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(latestRelease, filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{{
			ServiceType:        "nssm",
			WindowsServiceName: "api",
			BinaryName:         "api.exe",
			EnvFile:            ".env",
			EnvContent:         "VERSION=old\n",
		}},
	}
	if err := CaptureConfigSnapshot(wcfg, "v1", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	wcfg.Services[0].EnvContent = "VERSION=latest-updated\n"
	if err := CaptureConfigSnapshot(wcfg, "v2", SnapshotSourceActiveUpdate); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("VERSION=latest-updated\n"), 0600); err != nil {
		t.Fatal(err)
	}

	installNSSMCommandMock(t)
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	deployer := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	deployer.serviceManager = manager
	if err := deployer.Rollback(context.Background(), "v1"); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "VERSION=old\n"; got != want {
		t.Fatalf("environment after rollback = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "current", legacyConfigSnapshotDir)); !os.IsNotExist(err) {
		t.Fatalf("legacy snapshot exposed through current: %v", err)
	}
}

func TestManualRollbackRejectsMissingSnapshotBeforeStoppingServices(t *testing.T) {
	root := t.TempDir()
	targetRelease := filepath.Join(root, "releases", "v1")
	if err := os.MkdirAll(targetRelease, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetRelease, "api.exe"), []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}
	manager := &fakeServiceManager{
		states:      map[string]ServiceState{"api": ServiceStateRunning},
		stopErrors:  map[string]error{},
		startErrors: map[string]error{},
	}
	wcfg := &WatcherConfig{
		InstallDir: root,
		Services: []ServiceConfig{{
			ServiceType:        "nssm",
			WindowsServiceName: "api",
			BinaryName:         "api.exe",
		}},
	}
	deployer := NewDeployer(wcfg, "nssm.exe", newTestLogger(), nil)
	deployer.serviceManager = manager
	err := deployer.Rollback(context.Background(), "v1")
	if !errors.Is(err, ErrConfigSnapshotNotFound) || !strings.Contains(err.Error(), "validate rollback target config") {
		t.Fatalf("rollback error = %v, want missing snapshot preflight error", err)
	}
	if len(manager.calls) != 0 {
		t.Fatalf("service lifecycle calls = %v, want none", manager.calls)
	}
}
