package agent

import (
	"net/http"
	"time"

	"github.com/google/uuid"
)

// tracedTransport logs request headers/status timing without bodies, tokens, or URL queries.
type tracedTransport struct {
	base http.RoundTripper
	log  *Logger
}

// NewTraceTransport decorates an HTTP transport with request and operation IDs.
func NewTraceTransport(base http.RoundTripper, log *Logger) http.RoundTripper {
	return tracedTransport{base: base, log: log}
}

func (t tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	id := uuid.NewString()
	trace := TraceFromContext(req.Context())
	request.Header.Set("X-Request-ID", id)
	if trace.CorrelationID != "" {
		request.Header.Set("X-Correlation-ID", trace.CorrelationID)
	}
	if trace.PollID != "" {
		request.Header.Set("X-Poll-ID", trace.PollID)
	}
	url := *req.URL
	url.User, url.RawQuery, url.Fragment = nil, "", ""
	fields := []any{"network_request_id", id, "method", req.Method, "url", url.String()}
	logger := t.log.WithTrace(trace)
	logger.Info("HTTP request started", fields...)
	started := time.Now()
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(request)
	fields = append(fields, "duration_ms", time.Since(started).Milliseconds())
	if resp != nil {
		fields = append(fields, "http_status", resp.StatusCode, "remote_request_id", resp.Header.Get("X-GitHub-Request-ID"))
	}
	if err != nil {
		logger.Error("HTTP request failed", fields...)
	} else if resp.StatusCode >= 400 {
		logger.Warn("HTTP request completed", fields...)
	} else {
		logger.Info("HTTP request completed", fields...)
	}
	return resp, err
}
