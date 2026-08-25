package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateEnvFileRejectsEmptyPath(t *testing.T) {
	err := UpdateEnvFile("", map[string]string{"LOG_LEVEL": "debug"})
	if err == nil || !strings.Contains(err.Error(), "env path is empty") {
		t.Fatalf("UpdateEnvFile() error = %v", err)
	}
}

func TestUpdateEnvFileCreatesOrderedSecureFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	updates := map[string]string{
		"API_PORT":                       "9090",
		"ENVIRONMENT":                    "staging",
		"WEBHOOK_DEFAULT_SIGNING_SECRET": "signing-secret",
		"CUSTOM_SETTING":                 "preserved",
	}

	if err := UpdateEnvFile(path, updates); err != nil {
		t.Fatalf("UpdateEnvFile() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	want := strings.Join([]string{
		"ENVIRONMENT=staging",
		"WEBHOOK_DEFAULT_SIGNING_SECRET=signing-secret",
		"API_PORT=9090",
		"CUSTOM_SETTING=preserved",
		"",
	}, "\n")
	if string(content) != want {
		t.Errorf("env content:\n%s\nwant:\n%s", content, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat env file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("env mode = %o, want 600", got)
	}
}

func TestUpdateEnvFilePreservesCommentsUnknownEntriesAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := strings.Join([]string{
		"# watcher settings",
		"CUSTOM_EXISTING=keep-me",
		"LOG_LEVEL=info",
		"MALFORMED LINE",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatalf("write env fixture: %v", err)
	}

	err := UpdateEnvFile(path, map[string]string{
		"LOG_LEVEL":       "error",
		"LOG_MAX_SIZE_MB": "50",
	})
	if err != nil {
		t.Fatalf("UpdateEnvFile() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	want := strings.Join([]string{
		"# watcher settings",
		"CUSTOM_EXISTING=keep-me",
		"LOG_LEVEL=error",
		"MALFORMED LINE",
		"",
		"LOG_MAX_SIZE_MB=50",
		"",
	}, "\n")
	if string(content) != want {
		t.Errorf("env content:\n%s\nwant:\n%s", content, want)
	}
}

func TestUpdateEnvFileReplacesDuplicateKnownEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("LOG_LEVEL=info\nLOG_LEVEL=warn\n"), 0600); err != nil {
		t.Fatalf("write env fixture: %v", err)
	}

	if err := UpdateEnvFile(path, map[string]string{"LOG_LEVEL": "debug"}); err != nil {
		t.Fatalf("UpdateEnvFile() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if got, want := string(content), "LOG_LEVEL=debug\nLOG_LEVEL=debug\n"; got != want {
		t.Errorf("env content = %q, want %q", got, want)
	}
}

func TestUpdateEnvFileReturnsReadError(t *testing.T) {
	err := UpdateEnvFile(t.TempDir(), map[string]string{"LOG_LEVEL": "debug"})
	if err == nil || !strings.Contains(err.Error(), "read env file") {
		t.Fatalf("UpdateEnvFile(directory) error = %v, want read error", err)
	}
}
