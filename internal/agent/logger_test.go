package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestWatcherLoggerIsolationAndRename(t *testing.T) {
	dir := t.TempDir()
	log, err := newFileLogger("agent", dir, LogConfig{Level: "debug", MaxSizeMB: 1}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	first := log.WithWatcher(7, "same name")
	second := log.WithWatcher(8, "same name")
	first.Info("first poll")
	second.Error("second poll failed", "error", "timeout")
	log.WithWatcher(7, "renamed").WithComponent("rollback").Warn("manual rollback")
	log.Info("global only")
	data, err := os.ReadFile(WatcherLogPath(dir, 7))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "second poll") || strings.Contains(string(data), "global only") {
		t.Fatalf("cross-watcher records: %s", data)
	}
	if !strings.Contains(string(data), "first poll") || !strings.Contains(string(data), "manual rollback") {
		t.Fatalf("records lost after rename: %s", data)
	}
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Count(raw, `"component":`) != 1 || strings.Count(raw, `"watcher_id":`) != 1 {
			t.Fatalf("duplicate identity fields: %s", raw)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			t.Fatal(err)
		}
		if record["watcher_id"] != float64(7) {
			t.Fatalf("wrong identity: %v", record)
		}
	}
	global, err := os.ReadFile(filepath.Join(dir, LogFilename))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(global), "\n") != 4 {
		t.Fatalf("global records=%s", global)
	}
}

func TestWatcherLogConcurrentAppends(t *testing.T) {
	dir := t.TempDir()
	log, err := newFileLogger("agent", dir, LogConfig{Level: "info", MaxSizeMB: 1}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			log.WithComponent("api").WithWatcher(42, "poller").Info("check", "sequence", i)
		}(i)
	}
	wg.Wait()
	data, err := os.ReadFile(WatcherLogPath(dir, 42))
	if err != nil {
		t.Fatal(err)
	}
	records := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(records) != 40 {
		t.Fatalf("records=%d", len(records))
	}
	for _, raw := range records {
		if !json.Valid([]byte(raw)) {
			t.Fatalf("interleaved record: %s", raw)
		}
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
	scoped := log.WithWatcher(9, "poller")
	scoped.Info("first log entry", "payload", payload)
	scoped.Info("second log entry", "payload", payload)

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
	watcherDir := filepath.Dir(WatcherLogPath(dir, 9))
	watcherRotated, err := filepath.Glob(filepath.Join(watcherDir, "watcher-*.log"))
	if err != nil || len(watcherRotated) == 0 {
		t.Fatalf("watcher rotation: files=%v error=%v", watcherRotated, err)
	}
	active, err := os.ReadFile(WatcherLogPath(dir, 9))
	if err != nil || !bytes.Contains(active, []byte("second log entry")) || bytes.Contains(active, []byte("first log entry")) {
		t.Fatalf("watcher active file failed to rotate: error=%v", err)
	}
}
