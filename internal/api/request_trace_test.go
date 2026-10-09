package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestRequestTraceManualTriggerAndQueueFull(t *testing.T) {
	db, err := database.NewDB(filepath.Join(t.TempDir(), "trace.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "poller", ServiceName: "app"}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	queue := make(chan agent.CheckTrigger, 1)
	h := &Handler{db: db, checkTrigger: queue, log: agent.NewLoggerWithWriter("api", &logs, "info")}
	router := gin.New()
	router.Use(h.RequestTrace())
	router.POST("/watchers/:id/check", h.TriggerCheck)
	id := uuid.NewString()
	request := httptest.NewRequest("POST", "/watchers/1/check?token=secret", nil)
	request.Header.Set("X-Request-ID", id)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	var body agent.Trace
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusAccepted || recorder.Header().Get("X-Request-ID") != id || body.RequestID != id || body.CorrelationID != id || body.PollID == "" || recorder.Header().Get("X-Poll-ID") != body.PollID {
		t.Fatalf("response=%d %s", recorder.Code, recorder.Body)
	}
	queued := <-queue
	if queued.WatcherID != watcher.ID || queued.Trace.PollID != body.PollID || queued.Trace.RequestID != id || queued.Trace.TriggeredBy != "manual" {
		t.Fatalf("queue=%+v", queued)
	}

	queue <- queued
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest("POST", "/watchers/1/check", nil)
	request.Header.Set("X-Request-ID", "invalid")
	router.ServeHTTP(recorder, request)
	if recorder.Code != 503 || recorder.Header().Get("X-Poll-ID") != "" {
		t.Fatalf("full queue response=%d %s", recorder.Code, recorder.Body)
	}
	if _, err := uuid.Parse(recorder.Header().Get("X-Request-ID")); err != nil {
		t.Fatal("missing generated request ID")
	}
	if strings.Contains(logs.String(), "secret") {
		t.Fatal("query credentials logged")
	}
}

func TestTraceLogFilterFindsOlderRunBeforeLimiting(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	path := filepath.Join(dir, agent.LogFilename)
	content := `{"msg":"matching first","poll_id":"` + id + `"}` + "\n" + strings.Repeat("{\"msg\":\"unrelated\"}\n", 1500) + `{"msg":"matching last","correlation_id":"` + id + `"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	h := &Handler{logDir: dir}
	router := gin.New()
	router.GET("/logs", h.AgentLogs)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/logs?lines=2&trace_id="+id, nil))
	var result LogFileResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != 200 || result.TraceID != id || len(result.Lines) != 2 || !strings.Contains(result.Lines[0], "matching first") {
		t.Fatalf("filtered logs=%d %+v", recorder.Code, result)
	}
}

func TestTraceLogFilterReadsRetainedArchives(t *testing.T) {
	dir := t.TempDir()
	id := uuid.NewString()
	path := filepath.Join(dir, agent.LogFilename)
	archive, err := os.Create(filepath.Join(dir, "watcher-2026-10-09T10-00-00.000.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(archive)
	if _, err := writer.Write([]byte(`{"msg":"archived failure","poll_id":"` + id + `"}` + "\n")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"msg":"active failure","poll_id":"`+id+`"}`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := tailTraceFile(context.Background(), path, id, 2)
	if err != nil || len(lines) != 2 || !strings.Contains(lines[0], "archived failure") || !strings.Contains(lines[1], "active failure") {
		t.Fatalf("lines=%v error=%v", lines, err)
	}
}
