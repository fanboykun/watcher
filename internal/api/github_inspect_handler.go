package api

import (
	"net/http"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

type inspectRequest struct {
	SourceType  string `json:"source_type"`
	RepoURL     string `json:"repo_url" binding:"required"`
	ReleaseRef  string `json:"release_ref"`
	GitHubToken string `json:"github_token"`
	// Feed credential for previewing an authenticated http feed.
	FeedAuthType     string `json:"feed_auth_type"`
	FeedAuthUsername string `json:"feed_auth_username"`
	FeedAuthHeader   string `json:"feed_auth_header"`
	FeedAuthRegion   string `json:"feed_auth_region"`
	FeedAuthSecret   string `json:"feed_auth_secret"`
}

// InspectGitHubRepo previews a release source's services for the setup wizard.
// GitHub sources use the configured token; feeds never receive it.
func (h *Handler) InspectGitHubRepo(c *gin.Context) {
	var req inspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}
	sourceType, _, err := resolveWatcherSource(req.SourceType, req.RepoURL, req.ReleaseRef)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	feedAuth, err := resolveFeedAuth(sourceType, agent.FeedAuth{Type: req.FeedAuthType, Username: req.FeedAuthUsername, Header: req.FeedAuthHeader, Region: req.FeedAuthRegion, Secret: req.FeedAuthSecret})
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if sourceType != agent.SourceGitHub {
		resp, err := agent.InspectFeed(c.Request.Context(), sourceType, req.RepoURL, req.ReleaseRef, feedAuth, h.log.WithComponent("feed-inspect"))
		if err != nil {
			c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	token := strings.TrimSpace(req.GitHubToken)
	if token == "" {
		token = h.githubToken
	}
	client := agent.NewGitHubClient(token, h.log.WithComponent("github-inspect"))

	resp, err := client.InspectRepository(c.Request.Context(), req.RepoURL, req.ReleaseRef)
	if err != nil {
		githubError(c, http.StatusInternalServerError, err)
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SyncServiceEnv updates the .env content for a service and syncs it to disk.
