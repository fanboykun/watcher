package agent

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ============================================================
// isNewer — version comparison
// ============================================================

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest   string
		current  string
		expected bool
	}{
		// Standard cases
		{"v2.0.0", "v1.0.0", true},
		{"v1.1.0", "v1.0.0", true},
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.0", false},
		{"v1.0.0", "v2.0.0", false},
		{"v1.0.0", "v1.1.0", false},

		// Without "v" prefix
		{"2.0.0", "1.0.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.0.0", "2.0.0", false},

		// Mixed prefix
		{"v2.0.0", "1.0.0", true},
		{"2.0.0", "v1.0.0", true},

		// Dev/empty
		{"v1.0.0", "dev", true},
		{"v1.0.0", "", true},
		{"", "", false},

		// Multi-digit
		{"v1.10.0", "v1.9.0", true},
		{"v1.9.0", "v1.10.0", false},
		{"v2.0.0", "v1.99.99", true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s_vs_%s", tt.latest, tt.current)
		t.Run(name, func(t *testing.T) {
			got := isNewer(tt.latest, tt.current)
			if got != tt.expected {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.expected)
			}
		})
	}
}

// ============================================================
// extractFileFromZip
// ============================================================

func TestExtractFileFromZip_Success(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	// Create a test zip
	createTestZipFile(t, zipPath, map[string]string{
		"watcher.exe": "binary content here",
		"README.md":   "readme content",
		"config.json": `{"key": "value"}`,
	})

	destPath := filepath.Join(dir, "extracted.exe")
	err := extractFileFromZip(zipPath, "watcher.exe", destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(content) != "binary content here" {
		t.Errorf("content = %q, want %q", string(content), "binary content here")
	}
}

func TestExtractFileFromZip_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	createTestZipFile(t, zipPath, map[string]string{
		"other.exe": "something",
	})

	destPath := filepath.Join(dir, "extracted.exe")
	err := extractFileFromZip(zipPath, "watcher.exe", destPath)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if got := err.Error(); got != `file "watcher.exe" not found in zip` {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtractFileFromZip_NestedFile(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "test.zip")

	// Create a zip with nested directory structure
	createTestZipFile(t, zipPath, map[string]string{
		"subdir/watcher.exe": "nested binary",
	})

	destPath := filepath.Join(dir, "extracted.exe")
	err := extractFileFromZip(zipPath, "watcher.exe", destPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if string(content) != "nested binary" {
		t.Errorf("content = %q, want %q", string(content), "nested binary")
	}
}

func TestExtractFileFromZip_InvalidZip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bad.zip")
	os.WriteFile(zipPath, []byte("not a zip"), 0644)

	err := extractFileFromZip(zipPath, "watcher.exe", filepath.Join(dir, "out.exe"))
	if err == nil {
		t.Fatal("expected error for invalid zip, got nil")
	}
}

// ============================================================
// GenerateUninstallScript
// ============================================================

func TestGenerateUninstallScript(t *testing.T) {
	script := GenerateUninstallScript(`C:\nssm.exe`, "WatcherAgent", `D:\apps\watcher`)

	// Verify key elements are present
	checks := []string{
		`C:\nssm.exe`,
		"WatcherAgent",
		`D:\apps\watcher`,
		"stop",
		"remove",
		"Remove-Item",
	}
	for _, check := range checks {
		if !contains(script, check) {
			t.Errorf("script missing %q", check)
		}
	}
}

// ============================================================
// Helpers
// ============================================================

func createTestZipFile(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		fmt.Fprint(f, content)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
