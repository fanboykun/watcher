package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CreateDeployment creates deployment.
func (g *GitHubClient) CreateDeployment(ctx context.Context, owner, repo, ref, environment, description string) (int64, error) {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/deployments", g.apiBase, owner, repo)

	body := map[string]any{
		"ref":               ref,
		"environment":       environment,
		"description":       description,
		"auto_merge":        false,
		"required_contexts": []string{}, // skip status checks — we're deploying ourselves
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := g.newRequest(ctx, http.MethodPost, apiURL)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(string(bodyJSON)))

	resp, err := g.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("create deployment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		hint := githubDeployHint(resp.StatusCode)
		return 0, fmt.Errorf("create deployment returned HTTP %d: %s%s", resp.StatusCode, string(respBody), hint)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode deployment response: %w", err)
	}

	g.log.Info("GitHub deployment created", "deployment_id", result.ID, "ref", ref)
	return result.ID, nil
}

// UpdateDeploymentStatus updates the status of a GitHub deployment.
// Valid states: "pending", "in_progress", "success", "failure", "error", "inactive"
func (g *GitHubClient) UpdateDeploymentStatus(ctx context.Context, owner, repo string, deploymentID int64, state, logURL, description string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/deployments/%d/statuses", g.apiBase, owner, repo, deploymentID)

	body := map[string]string{
		"state":       state,
		"description": limitGitHubDeploymentDescription(description, githubDeploymentStatusDescriptionLimit),
	}
	if logURL != "" {
		body["log_url"] = logURL
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := g.newRequest(ctx, http.MethodPost, apiURL)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	req.Body = io.NopCloser(strings.NewReader(string(bodyJSON)))

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("update deployment status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		hint := githubDeployHint(resp.StatusCode)
		return fmt.Errorf("update deployment status returned HTTP %d: %s%s", resp.StatusCode, string(respBody), hint)
	}

	g.log.Debug("deployment status updated", "deployment_id", deploymentID, "state", state)
	return nil
}

// limitGitHubDeploymentDescription truncates a GitHub deployment description to the API rune limit.
func limitGitHubDeploymentDescription(description string, maxRunes int) string {
	description = strings.TrimSpace(description)
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(description)
	if len(runes) <= maxRunes {
		return description
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

// githubDeployHint returns an actionable hint for common GitHub Deployment API failures.
func githubDeployHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return " (hint: check GITHUB_TOKEN validity and scopes)"
	case http.StatusForbidden:
		return " (hint: token may lack Deployments write permission)"
	case http.StatusNotFound:
		return " (hint: owner/repo/ref may be wrong, or token cannot access the repo)"
	case http.StatusUnprocessableEntity:
		return " (hint: ref may not exist in repo, environment/log_url may be invalid, or branch protection blocks deployments)"
	default:
		return ""
	}
}

// FetchLatestRelease fetches the latest release info from a GitHub repository.
// Used for self-update checks.
