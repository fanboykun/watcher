package api

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ── List all services (flat, across all watchers) ─────────────────────

func (h *Handler) ListAllServices(c *gin.Context) {
	var services []database.Service
	if err := h.db.Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	type serviceWithWatcher struct {
		database.Service
		WatcherName string `json:"watcher_name"`
		InstallDir  string `json:"install_dir"`
	}

	var result []serviceWithWatcher
	for _, svc := range services {
		var watcher database.Watcher
		h.db.Select("name", "install_dir").First(&watcher, svc.WatcherID)
		result = append(result, serviceWithWatcher{
			Service:     svc,
			WatcherName: watcher.Name,
			InstallDir:  watcher.InstallDir,
		})
	}
	c.JSON(http.StatusOK, result)
}

// ── Service start/stop/restart ────────────────────────────────────────

func (h *Handler) StartService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	out, errExec := exec.Command(h.nssmPath, "start", svc.WindowsServiceName).CombinedOutput()
	if errExec != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("nssm start %s: %s (output: %s)", svc.WindowsServiceName, errExec, string(out)),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s started", svc.WindowsServiceName)})
}

func (h *Handler) StopService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	out, errExec := exec.Command(h.nssmPath, "stop", svc.WindowsServiceName, "confirm").CombinedOutput()
	if errExec != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("nssm stop %s: %s (output: %s)", svc.WindowsServiceName, errExec, string(out)),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s stopped", svc.WindowsServiceName)})
}

func (h *Handler) RestartService(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}
	if err := h.requireWindows(c); err != nil {
		return
	}

	// Stop
	exec.Command(h.nssmPath, "stop", svc.WindowsServiceName, "confirm").CombinedOutput()
	time.Sleep(2 * time.Second)

	// Start
	out, errExec := exec.Command(h.nssmPath, "start", svc.WindowsServiceName).CombinedOutput()
	if errExec != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("nssm start %s after restart: %s (output: %s)", svc.WindowsServiceName, errExec, string(out)),
		})
		return
	}
	c.JSON(http.StatusOK, MessageResponse{Message: fmt.Sprintf("service %s restarted", svc.WindowsServiceName)})
}

// ── Health status ─────────────────────────────────────────────────────

func (h *Handler) GetServiceHealth(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	if svc.HealthCheckURL == "" {
		c.JSON(http.StatusOK, gin.H{
			"service_id":     svc.ID,
			"service_name":   svc.WindowsServiceName,
			"status":         "unknown",
			"message":        "no health check URL configured",
		})
		return
	}

	// Perform live health check
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(svc.HealthCheckURL)

	event := database.HealthEvent{
		ServiceID: svc.ID,
		CheckedAt: timeNow(),
	}

	if err != nil {
		event.Status = "error"
		event.Error = err.Error()
	} else {
		event.HTTPStatus = resp.StatusCode
		resp.Body.Close()
		if resp.StatusCode == 200 {
			event.Status = "healthy"
		} else {
			event.Status = "unhealthy"
		}
	}

	// Record the event
	h.db.Create(&event)

	c.JSON(http.StatusOK, gin.H{
		"service_id":     svc.ID,
		"service_name":   svc.WindowsServiceName,
		"health_url":     svc.HealthCheckURL,
		"status":         event.Status,
		"http_status":    event.HTTPStatus,
		"error":          event.Error,
		"checked_at":     event.CheckedAt,
	})
}

func (h *Handler) GetHealthHistory(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	var events []database.HealthEvent
	if err := h.db.Where("service_id = ?", svc.ID).
		Order("id desc").Limit(limit).Find(&events).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

// ── Service logs ──────────────────────────────────────────────────────

func (h *Handler) GetServiceLogs(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	// Find the watcher's install_dir to locate log files
	var watcher database.Watcher
	if err := h.db.Select("install_dir").First(&watcher, svc.WatcherID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "parent watcher not found"})
		return
	}

	logType := c.DefaultQuery("type", "out") // "out" or "err"
	logFile := filepath.Join(watcher.InstallDir, "logs", svc.WindowsServiceName+"."+logType+".log")

	lines := 100
	if l := c.Query("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			lines = parsed
		}
	}

	content, err := tailFile(logFile, lines)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: fmt.Sprintf("log file not found: %s", logFile)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"service":  svc.WindowsServiceName,
		"log_file": logFile,
		"type":     logType,
		"lines":    content,
	})
}

// ── Service deploy history ────────────────────────────────────────────

func (h *Handler) GetServiceDeploys(c *gin.Context) {
	svc, err := h.findServiceByID(c)
	if err != nil {
		return
	}

	// Deploy logs are per-watcher, return all for this service's watcher
	var logs []database.DeployLog
	if err := h.db.Where("watcher_id = ?", svc.WatcherID).
		Order("id desc").Limit(50).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

// ── Helpers ───────────────────────────────────────────────────────────

func (h *Handler) findServiceByID(c *gin.Context) (*database.Service, error) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := h.db.First(&svc, id).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	return &svc, nil
}

func (h *Handler) requireWindows(c *gin.Context) error {
	if runtime.GOOS != "windows" {
		c.JSON(http.StatusServiceUnavailable, ErrorResponse{
			Error: fmt.Sprintf("NSSM service management only available on Windows (running on %s)", runtime.GOOS),
		})
		return fmt.Errorf("not windows")
	}
	return nil
}

// tailFile reads the last N lines from a file.
func tailFile(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var allLines []string
	scanner := bufio.NewScanner(f)
	// Increase buffer for potentially long log lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if len(allLines) <= n {
		return allLines, nil
	}
	return allLines[len(allLines)-n:], nil
}

// findServiceByWatcherAndID finds a service by watcher ID and service ID.
func (h *Handler) findServiceByWatcherAndID(c *gin.Context, db *gorm.DB) (*database.Service, error) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return nil, err
	}

	sid, err := strconv.ParseUint(c.Param("sid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid service id"})
		return nil, err
	}

	var svc database.Service
	if err := db.Where("id = ? AND watcher_id = ?", sid, watcher.ID).First(&svc).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "service not found"})
		return nil, err
	}
	return &svc, nil
}
