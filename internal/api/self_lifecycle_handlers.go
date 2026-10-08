package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
)

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

	exePath, err := os.Executable()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Could not locate watcher executable: " + err.Error()})
		return
	}
	if err := agent.ScheduleServiceRestart(h.nssmPath, svc, filepath.Dir(exePath)); err != nil {
		h.log.Error("could not schedule service restart", "error", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"message":      "watcher restart scheduled",
		"service_name": svc,
	})
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
