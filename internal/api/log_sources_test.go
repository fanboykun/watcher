package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

func TestLogSourcesAndWatcherScope(t *testing.T) {
	dir := t.TempDir()
	db, err := database.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "poller", ServiceName: "app", InstallDir: dir}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"agent":   filepath.Join(dir, agent.LogFilename),
		"stdout":  filepath.Join(dir, "custom-service-stdout.log"),
		"stderr":  filepath.Join(dir, "custom-service-stderr.log"),
		"watcher": agent.WatcherLogPath(dir, watcher.ID),
	}
	for source, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source+" first\n"+source+" last\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h := &Handler{db: db, logDir: dir, appCfg: &config.AppConfig{WatcherServiceName: "custom-watcher"}, isWindows: func() bool { return true }}
	h.runNSSMCommand = func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) != 3 || args[0] != "get" || args[1] != "custom-watcher" {
			t.Fatalf("unexpected NSSM call: %v", args)
		}
		source := "stdout"
		if args[2] == "AppStderr" {
			source = "stderr"
		} else if args[2] != "AppStdout" {
			t.Fatalf("setting=%s", args[2])
		}
		return []byte(paths[source] + "\r\n"), nil
	}
	router := gin.New()
	router.GET("/logs", h.AgentLogs)
	router.GET("/watchers/:id/logs", h.WatcherLogs)
	for _, source := range []string{"agent", "stdout", "stderr", "watcher"} {
		path := "/logs?lines=1&source=" + source
		if source == "watcher" {
			path = fmt.Sprintf("/watchers/%d/logs?lines=1", watcher.ID)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", source, rec.Code, rec.Body)
		}
		var got LogFileResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Source != source || got.LogFile != paths[source] || len(got.Lines) != 1 || got.Lines[0] != source+" last" {
			t.Fatalf("%s=%+v", source, got)
		}
	}
	for _, path := range []string{"/logs?source=../../secret", "/logs?lines=1001", "/watchers/invalid/logs", "/watchers/99999/logs"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 400 && rec.Code != 404 {
			t.Fatalf("invalid path %s status=%d", path, rec.Code)
		}
	}
	if err := os.Remove(paths["watcher"]); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/watchers/%d/logs", watcher.ID), nil))
	var missing LogFileResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &missing); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || !missing.Missing || missing.Lines == nil || len(missing.Lines) != 0 {
		t.Fatalf("missing file=%d %s", rec.Code, rec.Body)
	}
}

func TestLogTailLargeFilesAndPartialLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.log")
	content := strings.Repeat("old record\n", 20000) + "last one\r\nlast two"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailFile(path, 2)
	if err != nil || len(lines) != 2 || lines[0] != "last one" || lines[1] != "last two" {
		t.Fatalf("tail=%v error=%v", lines, err)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	lines, err = tailFile(path, 2)
	if err != nil || lines == nil || len(lines) != 0 {
		t.Fatalf("empty tail=%v error=%v", lines, err)
	}
}

func TestLogNSSMUnicodePath(t *testing.T) {
	path := `C:\apps\日本語\watcher.err.log`
	chars := utf16.Encode([]rune(path + "\r\n"))
	out := make([]byte, 2+len(chars)*2)
	out[0], out[1] = 0xff, 0xfe
	for i, char := range chars {
		binary.LittleEndian.PutUint16(out[2+i*2:], char)
	}
	if got := decodeNSSMPath(out); got != path {
		t.Fatalf("path=%q", got)
	}
	if got := decodeNSSMPath(out[2:]); got != path {
		t.Fatalf("path without BOM=%q", got)
	}
}
