package api

import (
	"net/http"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

type githubQuotaEntry struct {
	Source     string `json:"source"` // "global", "anonymous" or "watcher" (a watcher token override)
	WatcherIDs []uint `json:"watcher_ids"`
	agent.GitHubQuota
}

// GitHubRateLimits reports the shared GitHub quota per configured token. Tokens are
// grouped in memory only; the response never contains token material.
func (h *Handler) GitHubRateLimits(c *gin.Context) {
	var watchers []database.Watcher
	if err := h.db.Select("id", "github_token").Order("id").Find(&watchers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	global := strings.TrimSpace(h.githubToken)
	source := "global"
	if global == "" {
		source = "anonymous"
	}
	// Self-update and inspection use the global token, so it is always listed.
	entries := []githubQuotaEntry{{Source: source, WatcherIDs: []uint{}}}
	tokens := []string{global}
	index := map[string]int{global: 0}
	for _, watcher := range watchers {
		token := strings.TrimSpace(watcher.GitHubToken)
		if token == "" {
			token = global
		}
		i, ok := index[token]
		if !ok {
			i = len(entries)
			index[token] = i
			tokens = append(tokens, token)
			entries = append(entries, githubQuotaEntry{Source: "watcher", WatcherIDs: []uint{}})
		}
		entries[i].WatcherIDs = append(entries[i].WatcherIDs, watcher.ID)
	}
	for i, token := range tokens {
		entries[i].GitHubQuota = agent.GitHubQuotaFor(token)
	}
	c.JSON(http.StatusOK, entries)
}
