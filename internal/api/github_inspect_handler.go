package api

import (
	"net/http"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

type inspectRequest struct {
	RepoURL     string `json:"repo_url" binding:"required"`
	ReleaseRef  string `json:"release_ref"`
	GitHubToken string `json:"github_token"`
}

// InspectGitHubRepo uses the configured GitHub token to check a repository's latest release.
func (h *Handler) InspectGitHubRepo(c *gin.Context) {
	var req inspectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid request payload"})
		return
	}

	token := strings.TrimSpace(req.GitHubToken)
	if token == "" {
		token = h.githubToken
	}
	client := agent.NewGitHubClient(token, h.log.WithComponent("github-inspect"))

	resp, err := client.InspectRepository(c.Request.Context(), req.RepoURL, req.ReleaseRef)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// SyncServiceEnv updates the .env content for a service and syncs it to disk.
