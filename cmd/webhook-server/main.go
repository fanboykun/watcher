package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	standardwebhooks "github.com/fanboykun/watcher/internal/webhook"
)

type receivedWebhook struct {
	ID          int               `json:"id"`
	ReceivedAt  time.Time         `json:"received_at"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	RemoteAddr  string            `json:"remote_addr"`
	EventType   string            `json:"event_type"`
	DeliveryID  string            `json:"delivery_id"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
}

type serverConfig struct {
	Addr      string
	Path      string
	Secret    string
	Status    int
	MaxEvents int
}

type webhookStore struct {
	mu     sync.Mutex
	nextID int
	events []receivedWebhook
	max    int
}

func newWebhookStore(max int) *webhookStore {
	if max < 1 {
		max = 100
	}
	return &webhookStore{max: max}
}

func (s *webhookStore) add(event receivedWebhook) receivedWebhook {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	event.ID = s.nextID
	s.events = append([]receivedWebhook{event}, s.events...)
	if len(s.events) > s.max {
		s.events = s.events[:s.max]
	}
	return event
}

func (s *webhookStore) list() []receivedWebhook {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.events)
}

type pageData struct {
	Config serverConfig
	Events []receivedWebhook
}

var indexTemplate = template.Must(template.New("index").Funcs(template.FuncMap{
	"prettyTime": func(t time.Time) string {
		return t.Format(time.RFC3339)
	},
	"shortBody": func(body string) string {
		body = strings.TrimSpace(body)
		if len(body) <= 400 {
			return body
		}
		return body[:400] + "..."
	},
}).Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Watcher Webhook Server</title>
  <style>
    :root { color-scheme: dark; }
    body { font-family: ui-sans-serif, system-ui, sans-serif; background: #0b1220; color: #e5eefb; margin: 0; }
    main { max-width: 1100px; margin: 0 auto; padding: 32px 20px 48px; }
    .card { background: #121a2a; border: 1px solid #24324a; border-radius: 14px; padding: 18px; margin-bottom: 18px; }
    .muted { color: #95a7c4; }
    .mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    .grid { display: grid; gap: 12px; }
    .pill { display: inline-block; padding: 4px 8px; border-radius: 999px; border: 1px solid #31517a; background: #13233c; color: #a8d1ff; font-size: 12px; }
    pre { white-space: pre-wrap; word-break: break-word; background: #0f1726; border: 1px solid #21314c; border-radius: 10px; padding: 12px; overflow: auto; }
    table { width: 100%; border-collapse: collapse; }
    th, td { text-align: left; padding: 10px 8px; border-bottom: 1px solid #223149; vertical-align: top; }
    th { color: #95a7c4; font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
    a { color: #8dc2ff; }
  </style>
</head>
<body>
  <main>
    <div class="card">
      <h1 style="margin:0 0 8px;">Watcher Webhook Server</h1>
      <p class="muted" style="margin:0 0 14px;">
        Minimal receiver for testing Watcher webhooks. Point a watcher webhook URL at this server and inspect the captured requests below.
      </p>
      <div class="grid">
        <div><strong>POST endpoint:</strong> <span class="mono">{{.Config.Path}}</span></div>
        <div><strong>Events JSON:</strong> <a href="/events" class="mono">/events</a></div>
        <div><strong>Health:</strong> <a href="/healthz" class="mono">/healthz</a></div>
        <div><strong>Standard signature verification:</strong>
          {{if .Config.Secret}}<span class="pill">required</span>{{else}}<span class="pill">not required</span>{{end}}
        </div>
        <div><strong>Success response:</strong> HTTP {{.Config.Status}}</div>
      </div>
    </div>

    <div class="card">
      <h2 style="margin-top:0;">Recent Deliveries</h2>
      {{if .Events}}
      <table>
        <thead>
          <tr>
            <th>#</th>
            <th>When</th>
            <th>Event</th>
            <th>Delivery ID</th>
            <th>Remote</th>
            <th>Body Preview</th>
          </tr>
        </thead>
        <tbody>
          {{range .Events}}
          <tr>
            <td class="mono">{{.ID}}</td>
            <td class="mono">{{prettyTime .ReceivedAt}}</td>
            <td><span class="pill">{{if .EventType}}{{.EventType}}{{else}}unknown{{end}}</span></td>
            <td class="mono">{{.DeliveryID}}</td>
            <td class="mono">{{.RemoteAddr}}</td>
            <td><pre>{{shortBody .Body}}</pre></td>
          </tr>
          {{end}}
        </tbody>
      </table>
      {{else}}
      <p class="muted" style="margin-bottom:0;">No webhook deliveries received yet.</p>
      {{end}}
    </div>
  </main>
</body>
</html>`))

func main() {
	cfg := serverConfig{}
	flag.StringVar(&cfg.Addr, "addr", ":8091", "listen address")
	flag.StringVar(&cfg.Path, "path", "/webhook", "webhook POST path")
	flag.StringVar(&cfg.Secret, "secret", "", "optional Standard Webhooks signing secret (whsec_...) to require")
	flag.IntVar(&cfg.Status, "status", http.StatusOK, "HTTP status to return for accepted webhook requests")
	flag.IntVar(&cfg.MaxEvents, "max-events", 100, "maximum recent webhook events to keep in memory")
	flag.Parse()

	if !strings.HasPrefix(cfg.Path, "/") {
		cfg.Path = "/" + cfg.Path
	}
	if cfg.Status < 100 || cfg.Status > 599 {
		log.Fatalf("invalid -status %d", cfg.Status)
	}

	store := newWebhookStore(cfg.MaxEvents)
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"path":   cfg.Path,
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count":  len(store.list()),
			"events": store.list(),
		})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := indexTemplate.Execute(w, pageData{
			Config: cfg,
			Events: store.list(),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	mux.HandleFunc(cfg.Path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message":            "Watcher webhook receiver is ready",
				"post_to":            cfg.Path,
				"events_json":        "/events",
				"healthz":            "/healthz",
				"requires_signature": strings.TrimSpace(cfg.Secret) != "",
			})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.Secret != "" {
			wh, err := standardwebhooks.NewStandardWebhook(cfg.Secret)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid -secret: %v", err), http.StatusInternalServerError)
				return
			}
			payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
				return
			}
			r.Body.Close()
			if err := wh.Verify(payload, r.Header); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": err.Error(),
				})
				return
			}
			r.Body = io.NopCloser(strings.NewReader(string(payload)))
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
			return
		}

		headers := map[string]string{}
		for k, values := range r.Header {
			headers[k] = strings.Join(values, ", ")
		}

		event := store.add(receivedWebhook{
			ReceivedAt:  time.Now().UTC(),
			Method:      r.Method,
			Path:        r.URL.Path,
			RemoteAddr:  r.RemoteAddr,
			EventType:   r.Header.Get("X-Watcher-Event"),
			DeliveryID:  r.Header.Get("X-Watcher-Delivery-ID"),
			ContentType: r.Header.Get("Content-Type"),
			Headers:     headers,
			Body:        string(body),
		})

		log.Printf("received webhook id=%d event=%s delivery_id=%s remote=%s", event.ID, event.EventType, event.DeliveryID, event.RemoteAddr)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(cfg.Status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":          true,
			"id":          event.ID,
			"event_type":  event.EventType,
			"delivery_id": event.DeliveryID,
			"status":      cfg.Status,
		})
	})

	log.Printf("watcher webhook server listening on %s (POST %s)", cfg.Addr, cfg.Path)
	log.Printf("open http://127.0.0.1%s/ to inspect recent deliveries", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, mux); err != nil {
		log.Fatal(err)
	}
}
