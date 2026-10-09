package api

import (
	"net/http"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *Handler) RequestTrace() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if parsed, err := uuid.Parse(id); err == nil {
			id = parsed.String()
		} else {
			id = uuid.NewString()
		}
		trace := agent.Trace{RequestID: id, CorrelationID: id}
		c.Request = c.Request.WithContext(agent.WithTrace(c.Request.Context(), trace))
		c.Header("X-Request-ID", id)
		c.Header("X-Correlation-ID", id)
		started := time.Now()
		if h.log != nil {
			h.log.WithTrace(trace).Info("API request started", "method", c.Request.Method, "route", c.FullPath())
		}
		c.Next()
		if h.log != nil {
			trace.PollID = c.Writer.Header().Get("X-Poll-ID")
			logger := h.log.WithTrace(trace)
			fields := []any{"method", c.Request.Method, "route", c.FullPath(), "http_status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds()}
			if c.Writer.Status() >= 500 {
				logger.Error("API request completed", fields...)
			} else {
				logger.Info("API request completed", fields...)
			}
		}
	}
}

func (h *Handler) queueCheck(c *gin.Context, watcherID uint) (agent.Trace, bool) {
	trace := agent.NewPollTrace(agent.TraceFromContext(c.Request.Context()).RequestID, "manual")
	select {
	case h.checkTrigger <- agent.CheckTrigger{WatcherID: watcherID, Trace: trace}:
		c.Header("X-Poll-ID", trace.PollID)
		if h.log != nil {
			h.log.WithTrace(trace).Info("manual poll queued", "watcher_id", watcherID)
		}
		return trace, true
	default:
		if h.log != nil {
			h.log.WithTrace(trace).Warn("manual poll not queued", "watcher_id", watcherID)
		}
		return trace, false
	}
}

func (h *Handler) pollQueueUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "Polling queue is full or unavailable; no check was queued"})
}
