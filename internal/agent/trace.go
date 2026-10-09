package agent

import (
	"context"

	"github.com/google/uuid"
)

// Trace identifies a single operation and the API request that initiated it.
type Trace struct {
	RequestID     string `json:"request_id,omitempty"`
	PollID        string `json:"poll_id,omitempty"`
	CorrelationID string `json:"correlation_id"`
	TriggeredBy   string `json:"triggered_by,omitempty"`
}

type CheckTrigger struct {
	WatcherID uint
	CatalogID uint
	Trace     Trace
}

type traceContextKey struct{}

func NewPollTrace(requestID, source string) Trace {
	id := uuid.NewString()
	correlationID := requestID
	if correlationID == "" {
		correlationID = id
	}
	return Trace{RequestID: requestID, PollID: id, CorrelationID: correlationID, TriggeredBy: source}
}

func WithTrace(ctx context.Context, trace Trace) context.Context {
	return context.WithValue(ctx, traceContextKey{}, trace)
}

func TraceFromContext(ctx context.Context) Trace {
	trace, _ := ctx.Value(traceContextKey{}).(Trace)
	return trace
}

func (l *Logger) WithTrace(trace Trace) *Logger {
	child := *l
	child.closer = nil
	child.trace = trace
	child.rebuild()
	return &child
}
