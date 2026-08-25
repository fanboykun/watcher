package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerWritesStructuredJSON(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	log := NewLoggerWithWriter("deploy", &output, "debug")
	log.Info("deployment started", "version", "v1.2.3", "attempt", 2)

	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log entry: %v", err)
	}
	if got, want := entry["component"], "deploy"; got != want {
		t.Errorf("component = %v, want %v", got, want)
	}
	if got, want := entry["msg"], "deployment started"; got != want {
		t.Errorf("msg = %v, want %v", got, want)
	}
	if got, want := entry["version"], "v1.2.3"; got != want {
		t.Errorf("version = %v, want %v", got, want)
	}
	if got, want := entry["attempt"], float64(2); got != want {
		t.Errorf("attempt = %v, want %v", got, want)
	}
}

func TestFileLoggerRotates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	log, err := newFileLogger("test", dir, LogConfig{
		Level:      "info",
		MaxSizeMB:  1,
		MaxBackups: 2,
		MaxAgeDays: 1,
	}, io.Discard)
	if err != nil {
		t.Fatalf("new file logger: %v", err)
	}
	defer log.Close()

	payload := strings.Repeat("x", 700*1024)
	log.Info("first log entry", "payload", payload)
	log.Info("second log entry", "payload", payload)

	if _, err := os.Stat(filepath.Join(dir, LogFilename)); err != nil {
		entries, readErr := os.ReadDir(dir)
		t.Fatalf("active log file: %v (directory entries: %v, read error: %v)", err, entries, readErr)
	}
	rotated, err := filepath.Glob(filepath.Join(dir, "watcher-*.log"))
	if err != nil {
		t.Fatalf("find rotated logs: %v", err)
	}
	if len(rotated) == 0 {
		t.Fatal("expected a rotated log file")
	}
}
