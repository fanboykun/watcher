package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
)

// SystemStatus returns agent health, uptime, version, and summary counts.
func (h *Handler) SystemStatus(c *gin.Context) {
	uptime := time.Since(h.startTime)

	var watcherCount, serviceCount int64
	h.db.Model(&database.Watcher{}).Count(&watcherCount)
	h.db.Model(&database.Service{}).Count(&serviceCount)

	// Count recent deploys (last 24h)
	var deployCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	h.db.Model(&database.DeployLog{}).Where("started_at > ?", dayAgo).Count(&deployCount)

	c.JSON(http.StatusOK, gin.H{
		"status":         "running",
		"version":        h.version,
		"uptime_seconds": int(uptime.Seconds()),
		"uptime_human":   formatDuration(uptime),
		"watcher_count":  watcherCount,
		"service_count":  serviceCount,
		"deploys_24h":    deployCount,
	})
}

// AgentLogs returns the last N lines of the watcher agent log file.
func (h *Handler) AgentLogs(c *gin.Context) {
	lines := 100
	if l := c.Query("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			lines = parsed
		}
	}

	logType := c.DefaultQuery("type", "out") // "out" or "err"
	logFile := filepath.Join(h.logDir, "watcher."+logType+".log")

	content, err := tailFile(logFile, lines)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error: fmt.Sprintf("agent log file not found: %s", logFile),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"log_file": logFile,
		"type":     logType,
		"lines":    content,
	})
}

// StreamAgentLogs streams the agent logs using Server-Sent Events (SSE).
func (h *Handler) StreamAgentLogs(c *gin.Context) {
	logType := c.DefaultQuery("type", "out")
	logFile := filepath.Join(h.logDir, "watcher."+logType+".log")

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	f, err := os.Open(logFile)
	if err != nil {
		c.SSEvent("error", "could not open log file")
		return
	}
	defer f.Close()

	// Send the last 50 lines to populate context
	content, _ := tailFile(logFile, 50)
	for _, line := range content {
		c.SSEvent("message", line)
	}
	c.Writer.Flush()

	f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						// Wait for next tick
						break
					}
					c.SSEvent("error", err.Error())
					return
				}
				c.SSEvent("message", line)
				c.Writer.Flush()
			}
		}
	}
}

// TriggerCheck sends a watcher ID to the check trigger channel
// so the agent runs an immediate poll cycle.
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
func (h *Handler) SelfVersion(c *gin.Context) {
	exePath, _ := os.Executable()
	c.JSON(http.StatusOK, gin.H{
		"version":    h.version,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"executable": exePath,
	})
}

// SelfConfig returns the current agent configuration loaded from .env.
func (h *Handler) SelfConfig(c *gin.Context) {
	if h.appCfg == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "app config is not available"})
		return
	}

	c.JSON(http.StatusOK, SelfConfigResponse{
		Environment:         h.appCfg.Environment,
		GitHubDeployEnabled: h.appCfg.GitHubDeployEnabled,
		LogDir:              h.appCfg.LogDir,
		NssmPath:            h.appCfg.NssmPath,
		DBPath:              h.appCfg.DBPath,
		APIPort:             h.appCfg.APIPort,
		APIBaseURL:          h.appCfg.APIBaseURL,
		WatcherRepoURL:      h.appCfg.WatcherRepoURL,
		WatcherServiceName:  h.selfServiceName(),
		HasGitHubToken:      strings.TrimSpace(h.appCfg.GitHubToken) != "",
		GitHubTokenMasked:   maskToken(h.appCfg.GitHubToken),
		EnvPath:             h.envPath,
	})
}

// UpdateSelfConfig updates selected agent config values and persists them to .env.
// For fields used by running watcher loops, the agent goroutines are recreated.
func (h *Handler) UpdateSelfConfig(c *gin.Context) {
	if h.appCfg == nil {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "app config is not available"})
		return
	}
	var req UpdateSelfConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	next := *h.appCfg
	if req.Environment != nil {
		next.Environment = strings.TrimSpace(*req.Environment)
	}
	if req.GitHubToken != nil {
		next.GitHubToken = strings.TrimSpace(*req.GitHubToken)
	}
	if req.GitHubDeployEnabled != nil {
		next.GitHubDeployEnabled = *req.GitHubDeployEnabled
	}
	if req.LogDir != nil {
		next.LogDir = strings.TrimSpace(*req.LogDir)
	}
	if req.NssmPath != nil {
		next.NssmPath = strings.TrimSpace(*req.NssmPath)
	}
	if req.DBPath != nil {
		next.DBPath = strings.TrimSpace(*req.DBPath)
	}
	if req.APIPort != nil {
		port := strings.TrimSpace(*req.APIPort)
		if err := validatePort(port); err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
			return
		}
		next.APIPort = port
	}
	if req.APIBaseURL != nil {
		next.APIBaseURL = strings.TrimSpace(*req.APIBaseURL)
	}
	if req.WatcherRepoURL != nil {
		next.WatcherRepoURL = strings.TrimSpace(*req.WatcherRepoURL)
	}
	if req.WatcherServiceName != nil {
		next.WatcherServiceName = strings.TrimSpace(*req.WatcherServiceName)
	}

	updates := map[string]string{
		"ENVIRONMENT":           next.Environment,
		"GITHUB_TOKEN":          next.GitHubToken,
		"GITHUB_DEPLOY_ENABLED": strconv.FormatBool(next.GitHubDeployEnabled),
		"LOG_DIR":               next.LogDir,
		"NSSM_PATH":             next.NssmPath,
		"DB_PATH":               next.DBPath,
		"API_PORT":              next.APIPort,
		"API_BASE_URL":          next.APIBaseURL,
		"WATCHER_REPO_URL":      next.WatcherRepoURL,
		"WATCHER_SERVICE_NAME":  next.WatcherServiceName,
	}
	if err := config.UpdateEnvFile(h.envPath, updates); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	// Apply changes to in-memory runtime config.
	*h.appCfg = next
	h.githubToken = next.GitHubToken
	h.nssmPath = next.NssmPath
	h.logDir = next.LogDir

	// Recreate watcher goroutines so they pick up updated global values.
	h.touchWatchersUpdatedAt()
	h.triggerSync()

	c.JSON(http.StatusOK, gin.H{
		"message": "agent configuration saved",
		"notes": []string{
			"watcher loops were reloaded to apply runtime fields",
			"API_PORT and DB_PATH changes require manual service restart to fully take effect",
		},
		"config": SelfConfigResponse{
			Environment:         next.Environment,
			GitHubDeployEnabled: next.GitHubDeployEnabled,
			LogDir:              next.LogDir,
			NssmPath:            next.NssmPath,
			DBPath:              next.DBPath,
			APIPort:             next.APIPort,
			APIBaseURL:          next.APIBaseURL,
			WatcherRepoURL:      next.WatcherRepoURL,
			WatcherServiceName:  h.selfServiceName(),
			HasGitHubToken:      strings.TrimSpace(next.GitHubToken) != "",
			GitHubTokenMasked:   maskToken(next.GitHubToken),
			EnvPath:             h.envPath,
		},
	})
}

// SelfUpdateCheck checks for a newer version of the watcher from its GitHub repo.
func (h *Handler) SelfUpdateCheck(c *gin.Context) {
	if h.appCfg == nil || h.appCfg.WatcherRepoURL == "" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "WATCHER_REPO_URL not configured"})
		return
	}

	info, err := agent.CheckForUpdate(c.Request.Context(), h.version, h.appCfg.WatcherRepoURL, h.githubToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}

// SelfUpdate downloads and installs the latest watcher release.
func (h *Handler) SelfUpdate(c *gin.Context) {
	if h.appCfg == nil || h.appCfg.WatcherRepoURL == "" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "WATCHER_REPO_URL not configured"})
		return
	}

	// Check for update first
	info, err := agent.CheckForUpdate(c.Request.Context(), h.version, h.appCfg.WatcherRepoURL, h.githubToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	if !info.UpdateAvailable {
		c.JSON(http.StatusOK, gin.H{
			"message": "already up to date",
			"version": h.version,
		})
		return
	}

	if info.DownloadURL == "" {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "no download URL found in latest release"})
		return
	}

	// Perform the update (this may restart the process on Windows)
	if err := agent.PerformSelfUpdate(c.Request.Context(), info.DownloadURL, h.githubToken, h.nssmPath, h.selfServiceName()); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":     "update applied — restarting",
		"old_version": h.version,
		"new_version": info.LatestVersion,
	})
}

// SelfRestart restarts the watcher Windows service via NSSM.
func (h *Handler) SelfRestart(c *gin.Context) {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: fmt.Sprintf("self restart is only available on Windows (running on %s)", runtime.GOOS),
		})
		return
	}
	if strings.TrimSpace(h.nssmPath) == "" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "NSSM_PATH is not configured"})
		return
	}
	svc := h.selfServiceName()
	if svc == "" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "WATCHER_SERVICE_NAME is not configured"})
		return
	}

	// Respond first so the client can receive status before process restart affects connectivity.
	c.JSON(http.StatusAccepted, gin.H{
		"message":      "watcher restart triggered",
		"service_name": svc,
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		exec.Command(h.nssmPath, "restart", svc).CombinedOutput()
	}()
}

// SelfUninstall returns a PowerShell uninstall script for the watcher.
func (h *Handler) SelfUninstall(c *gin.Context) {
	exePath, _ := os.Executable()
	installDir := filepath.Dir(exePath)

	script := agent.GenerateUninstallScript(h.nssmPath, h.selfServiceName(), installDir)

	c.JSON(http.StatusOK, gin.H{
		"script":      script,
		"install_dir": installDir,
		"message":     "Run the script as Administrator to uninstall the watcher",
	})
}

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

func validatePort(port string) error {
	if port == "" {
		return fmt.Errorf("api_port cannot be empty")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("api_port must be a valid number between 1 and 65535")
	}
	return nil
}

func maskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return "********"
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func (h *Handler) selfServiceName() string {
	if h.appCfg != nil && strings.TrimSpace(h.appCfg.WatcherServiceName) != "" {
		return strings.TrimSpace(h.appCfg.WatcherServiceName)
	}
	return "app-watcher"
}
