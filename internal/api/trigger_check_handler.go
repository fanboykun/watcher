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

	// Non-blocking send — if buffer is full, the check is already pending
	select {
	case h.checkTrigger <- watcher.ID:
		c.JSON(http.StatusAccepted, MessageResponse{
			Message: fmt.Sprintf("immediate check triggered for watcher %q", watcher.Name),
		})
	default:
		c.JSON(http.StatusAccepted, MessageResponse{
			Message: fmt.Sprintf("check already pending for watcher %q", watcher.Name),
		})
	}
}

// ── Self-management endpoints ─────────────────────────────────────────

// SelfVersion returns the current watcher build info.
