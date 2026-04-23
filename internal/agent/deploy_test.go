package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ============================================================
// CleanOldReleases
// ============================================================

func TestCleanOldReleases_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create 5 version directories with staggered mod times
	versions := []string{"v1.0.0", "v1.1.0", "v1.2.0", "v1.3.0", "v1.4.0"}
	for i, v := range versions {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
		os.WriteFile(filepath.Join(vDir, "app.exe"), []byte("binary"), 0644)
		// Set mod times with increasing timestamps
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(vDir, modTime, modTime)
	}

	// Keep only 3
	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only 3 remain
	entries, _ := os.ReadDir(relDir)
	if len(entries) != 3 {
		t.Errorf("expected 3 directories, got %d", len(entries))
	}

	// The 3 newest should survive
	for _, v := range []string{"v1.2.0", "v1.3.0", "v1.4.0"} {
		if _, err := os.Stat(filepath.Join(relDir, v)); os.IsNotExist(err) {
			t.Errorf("expected %s to survive cleanup", v)
		}
	}

	// The 2 oldest should be gone
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		if _, err := os.Stat(filepath.Join(relDir, v)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", v)
		}
	}
}

func TestCleanOldReleases_FewerThanKeep(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Only 2 versions, keep=3 → nothing should be deleted
	for _, v := range []string{"v1.0.0", "v1.1.0"} {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
	}

	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, _ := os.ReadDir(relDir)
	if len(entries) != 2 {
		t.Errorf("expected 2 directories, got %d", len(entries))
	}
}

func TestCleanOldReleases_EmptyReleasesDir(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCleanOldReleases_NoReleasesDir(t *testing.T) {
	dir := t.TempDir()

	// No releases/ directory at all — should not error
	err := CleanOldReleases(dir, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================
// ListAvailableVersions
// ============================================================

func TestListAvailableVersions_SortedByModTime(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create versions with specific mod times (oldest to newest)
	versions := []string{"v1.0.0", "v1.1.0", "v1.2.0"}
	for i, v := range versions {
		vDir := filepath.Join(relDir, v)
		os.MkdirAll(vDir, 0755)
		os.WriteFile(filepath.Join(vDir, "app.exe"), []byte("binary content"), 0644)
		modTime := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(vDir, modTime, modTime)
	}

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(result))
	}

	// Should be sorted newest first
	if result[0].Version != "v1.2.0" {
		t.Errorf("first version = %q, want v1.2.0", result[0].Version)
	}
	if result[2].Version != "v1.0.0" {
		t.Errorf("last version = %q, want v1.0.0", result[2].Version)
	}

	// Verify size is computed
	for _, v := range result {
		if v.SizeBytes <= 0 {
			t.Errorf("version %s has no size", v.Version)
		}
	}
}

func TestListAvailableVersions_IgnoresFiles(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	os.MkdirAll(relDir, 0755)

	// Create a version directory and a stray file
	os.MkdirAll(filepath.Join(relDir, "v1.0.0"), 0755)
	os.WriteFile(filepath.Join(relDir, "leftover.zip"), []byte("zip"), 0644)

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 version (ignoring files), got %d", len(result))
	}
}

func TestListAvailableVersions_NoReleasesDir(t *testing.T) {
	dir := t.TempDir()

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for missing releases dir, got %v", result)
	}
}

func TestReleaseStorageName_RoundTrip(t *testing.T) {
	version := "alpha-api/v0.1.0"
	storage := releaseStorageName(version)
	if storage == version {
		t.Fatalf("expected storage name to escape path separators, got %q", storage)
	}
	if restored := restoreReleaseVersion(storage); restored != version {
		t.Fatalf("restored version = %q, want %q", restored, version)
	}
}

func TestListAvailableVersions_DecodesStoredVersionNames(t *testing.T) {
	dir := t.TempDir()
	relDir := filepath.Join(dir, "releases")
	if err := os.MkdirAll(relDir, 0755); err != nil {
		t.Fatalf("mkdir releases: %v", err)
	}

	rawVersion := "alpha-api/v0.1.0"
	storedDir := filepath.Join(relDir, releaseStorageName(rawVersion))
	if err := os.MkdirAll(storedDir, 0755); err != nil {
		t.Fatalf("mkdir version dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storedDir, "app.exe"), []byte("binary"), 0644); err != nil {
		t.Fatalf("write app file: %v", err)
	}

	result, err := ListAvailableVersions(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 version, got %d", len(result))
	}
	if result[0].Version != rawVersion {
		t.Fatalf("listed version = %q, want %q", result[0].Version, rawVersion)
	}
	if result[0].Path != storedDir {
		t.Fatalf("listed path = %q, want %q", result[0].Path, storedDir)
	}
}

func TestIISBindingFromPublicURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "http default port", raw: "http://example.com", want: "http/*:80:example.com"},
		{name: "http custom port", raw: "http://example.com:8080", want: "http/*:8080:example.com"},
		{name: "https default port", raw: "https://example.com", want: "https/*:443:example.com"},
		{name: "empty host allowed", raw: "http://:8080", want: "http/*:8080:"},
		{name: "missing scheme", raw: "example.com", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := iisBindingFromPublicURL(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got binding %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("binding = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureServiceByType_IISCreatesRegistration(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var calls []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		call := name
		if len(args) > 0 {
			call += " " + strings.Join(args, " ")
		}
		calls = append(calls, call)
		cmd := strings.Join(args, " ")
		switch cmd {
		case "list apppool my-pool":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		case "add apppool /name:my-pool":
			return []byte("APPPOOL object \"my-pool\" added"), nil
		case "set apppool my-pool /managedRuntimeVersion:v4.0":
			return []byte("APPPOOL object changed"), nil
		case "list site my-site":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		case "add site /name:my-site /bindings:http/*:8080:example.com /physicalPath:C:/apps/current":
			return []byte("SITE object \"my-site\" added"), nil
		case "set vdir my-site/ /physicalPath:C:/apps/current":
			return []byte("VDIR object changed"), nil
		case "set app my-site/ /applicationPool:my-pool":
			return []byte("APP object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		WindowsServiceName: "frontend",
		IISAppKind:         "aspnet_classic",
		IISSiteName:        "my-site",
		IISAppPool:         "my-pool",
		IISManagedRuntime:  "v4.0",
		PublicURL:          "http://example.com:8080",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}

	want := []string{
		appcmdPath() + " list apppool my-pool",
		appcmdPath() + " add apppool /name:my-pool",
		appcmdPath() + " set apppool my-pool /managedRuntimeVersion:v4.0",
		appcmdPath() + " list site my-site",
		appcmdPath() + " add site /name:my-site /bindings:http/*:8080:example.com /physicalPath:C:/apps/current",
		appcmdPath() + " set vdir my-site/ /physicalPath:C:/apps/current",
		appcmdPath() + " set app my-site/ /applicationPool:my-pool",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}

func TestEnsureServiceByType_IISStaticDefaultsAppPoolToNoManagedCode(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var sawRuntime bool
	runCommand = func(name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "list apppool my-pool":
			return []byte("APPPOOL \"my-pool\""), nil
		case "set apppool my-pool /managedRuntimeVersion:":
			sawRuntime = true
			return []byte("APPPOOL object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", strings.Join(args, " "))
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:       "iis",
		IISAppKind:        "static",
		IISAppPool:        "my-pool",
		IISManagedRuntime: "",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}
	if !sawRuntime {
		t.Fatal("expected app pool runtime to be configured")
	}
}

func TestEnsureServiceByType_IISRequiresPublicURLWhenCreatingSite(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	runCommand = func(name string, args ...string) ([]byte, error) {
		switch strings.Join(args, " ") {
		case "list site my-site":
			return []byte("ERROR ( message:Cannot find requested collection element )"), os.ErrNotExist
		default:
			return []byte(""), nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		IISAppKind:         "php",
		WindowsServiceName: "frontend",
		IISSiteName:        "my-site",
	}, "C:/apps/current")
	if err == nil {
		t.Fatal("expected error when public URL is missing for a new IIS site")
	}
}

func TestEnsureServiceByType_IISUpdatesExistingSiteWithoutPublicURL(t *testing.T) {
	origRunCommand := runCommand
	t.Cleanup(func() { runCommand = origRunCommand })

	var calls []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		call := name
		if len(args) > 0 {
			call += " " + strings.Join(args, " ")
		}
		calls = append(calls, call)
		switch strings.Join(args, " ") {
		case "list site my-site":
			return []byte("SITE \"my-site\""), nil
		case "set vdir my-site/ /physicalPath:C:/apps/current":
			return []byte("VDIR object changed"), nil
		default:
			t.Fatalf("unexpected command: %s", strings.Join(args, " "))
			return nil, nil
		}
	}

	d := NewDeployer(&WatcherConfig{}, "nssm", NewLogger("test"), nil)
	err := d.ensureServiceByType(ServiceConfig{
		ServiceType:        "iis",
		IISAppKind:         "static",
		WindowsServiceName: "frontend",
		IISSiteName:        "my-site",
	}, "C:/apps/current")
	if err != nil {
		t.Fatalf("ensureServiceByType returned error: %v", err)
	}

	want := []string{
		appcmdPath() + " list site my-site",
		appcmdPath() + " set vdir my-site/ /physicalPath:C:/apps/current",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call[%d] = %q, want %q", i, calls[i], want[i])
		}
	}
}
