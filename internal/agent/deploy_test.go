package agent

import (
	"os"
	"path/filepath"
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
