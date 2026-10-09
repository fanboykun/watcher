package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/google/uuid"
)

func TestAgentSupervisorPreservesQueuedPollTrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	dir := t.TempDir()
	db, err := database.NewDB(filepath.Join(dir, "supervisor.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "poller", ServiceName: "app", MetadataURL: server.URL + "/version.json", InstallDir: dir, CheckIntervalSec: 300}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	queue := make(chan CheckTrigger, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	log := NewLoggerWithWriter("agent", &bytes.Buffer{}, "error")
	controller := NewAgent(db, &config.AppConfig{}, log, nil, queue, make(chan struct{}, 1), nil)
	go func() { defer close(done); controller.Run(ctx) }()
	defer func() { cancel(); <-done }()
	trace := NewPollTrace(uuid.NewString(), "manual")
	queue <- CheckTrigger{WatcherID: watcher.ID, Trace: trace}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var event database.PollEvent
		if db.Where("poll_id = ?", trace.PollID).First(&event).Error == nil {
			if event.RequestID != trace.RequestID || event.CorrelationID != trace.CorrelationID || event.TriggeredBy != "manual" {
				t.Fatalf("supervisor lost trace: %+v", event)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("queued poll did not produce its correlated history record")
}

func TestPollTraceSurvivesMetadataFailureAndNextRun(t *testing.T) {
	var headers []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Clone())
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	db, err := database.NewDB(filepath.Join(t.TempDir(), "trace.db"))
	if err != nil {
		t.Fatal(err)
	}
	watcher := database.Watcher{Name: "poller", ServiceName: "app", MetadataURL: server.URL + "/version.json"}
	if err := db.Create(&watcher).Error; err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	runtime := NewRepoWatcher(&watcher, db, &config.AppConfig{}, NewLoggerWithWriter("agent", &output, "debug"), nil, nil)
	manual := NewPollTrace(uuid.NewString(), "manual")
	if runtime.Run(WithTrace(context.Background(), manual)) == nil {
		t.Fatal("expected metadata failure")
	}
	if runtime.Run(context.Background()) == nil {
		t.Fatal("expected scheduled failure")
	}
	var stored database.Watcher
	if err := db.First(&stored, watcher.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.LastPollStatus != "error" || stored.LastPollID == "" || stored.LastPollID == manual.PollID || stored.LastPollAt == nil || stored.LastPollError == "" {
		t.Fatalf("latest completed poll not persisted: %+v", stored)
	}
	var events []database.PollEvent
	if err := db.Order("id").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected one failure event per run: %+v", events)
	}
	if events[0].PollID != manual.PollID || events[0].RequestID != manual.RequestID || events[0].CorrelationID != manual.RequestID {
		t.Fatalf("manual trace lost: %+v", events[0])
	}
	if events[1].PollID == manual.PollID || events[1].PollID == "" || events[1].RequestID != "" || events[1].TriggeredBy != "scheduled" {
		t.Fatalf("trace leaked across runs: %+v", events[1])
	}
	if len(headers) != 2 || headers[0].Get("X-Poll-ID") != manual.PollID || headers[0].Get("X-Correlation-ID") != manual.RequestID || headers[1].Get("X-Poll-ID") != events[1].PollID {
		t.Fatalf("outbound headers=%v", headers)
	}
	for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(raw), &record); err != nil {
			t.Fatal(err)
		}
		if record["poll_id"] != manual.PollID && record["poll_id"] != events[1].PollID {
			t.Fatalf("unbound record: %s", raw)
		}
	}
}

func TestTraceTransportRedactsURLAndCorrelatesHeaders(t *testing.T) {
	var output bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Correlation-ID") == "" || r.Header.Get("X-Request-ID") == "" {
			t.Error("missing trace headers")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	trace := NewPollTrace("", "scheduled")
	request, err := http.NewRequestWithContext(WithTrace(context.Background(), trace), "GET", strings.Replace(server.URL, "://", "://name:secret@", 1)+"?token=private", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: tracedTransport{log: NewLoggerWithWriter("http", &output, "info")}}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if strings.Contains(output.String(), "secret") || strings.Contains(output.String(), "private") {
		t.Fatalf("credentials in transport log: %s", output.String())
	}
}
