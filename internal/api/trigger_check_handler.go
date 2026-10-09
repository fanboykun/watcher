package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) TriggerCheck(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}

	if h.checkTrigger == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "check trigger not available"})
		return
	}

	trace, queued := h.queueCheck(c, watcher.ID)
	if !queued {
		h.pollQueueUnavailable(c)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": fmt.Sprintf("immediate check triggered for watcher %q", watcher.Name), "request_id": trace.RequestID, "poll_id": trace.PollID, "correlation_id": trace.CorrelationID})
}

// ── Self-management endpoints ─────────────────────────────────────────

// SelfVersion returns the current watcher build info.
