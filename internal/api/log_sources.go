package api

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type LogFileResponse struct {
	Source    string   `json:"source"`
	WatcherID uint     `json:"watcher_id,omitempty"`
	LogFile   string   `json:"log_file"`
	Format    string   `json:"format"`
	Missing   bool     `json:"missing"`
	Lines     []string `json:"lines"`
	TraceID   string   `json:"trace_id,omitempty"`
}

func requestedLogLines(c *gin.Context) (int, error) {
	raw := c.DefaultQuery("lines", "100")
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 1000 {
		return 0, fmt.Errorf("lines must be between 1 and 1000")
	}
	return n, nil
}

func (h *Handler) agentLogSource(ctx context.Context, source string) (string, string, error) {
	switch source {
	case "agent":
		return filepath.Join(h.logDir, agent.LogFilename), "json", nil
	case "stdout", "stderr":
		suffix, setting := "out", "AppStdout"
		if source == "stderr" {
			suffix, setting = "err", "AppStderr"
		}
		fallback := filepath.Join(h.logDir, "watcher."+suffix+".log")
		if !h.runningOnWindows() || h.appCfg == nil || h.runNSSMCommand == nil {
			return fallback, "mixed", nil
		}
		service := strings.TrimSpace(h.appCfg.WatcherServiceName)
		if service == "" {
			return fallback, "mixed", nil
		}
		queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		out, err := h.runNSSMCommand(queryCtx, "get", service, setting)
		if err != nil {
			return "", "", fmt.Errorf("read Watcher service %s setting: %w (%s)", setting, err, decodeNSSMPath(out))
		}
		path := strings.Trim(decodeNSSMPath(out), "\"")
		if path == "" {
			return fallback, "mixed", nil
		}
		return path, "mixed", nil
	default:
		return "", "", fmt.Errorf("source must be agent, stdout, or stderr")
	}
}

func decodeNSSMPath(out []byte) string {
	// NSSM can print UTF-16LE when its output is redirected on Windows.
	if len(out) >= 2 && (out[0] == 0xff && out[1] == 0xfe || out[1] == 0) {
		if out[0] == 0xff && out[1] == 0xfe {
			out = out[2:]
		}
		chars := make([]uint16, len(out)/2)
		for i := range chars {
			chars[i] = binary.LittleEndian.Uint16(out[2*i:])
		}
		return strings.TrimSpace(string(utf16.Decode(chars)))
	}
	return strings.TrimSpace(string(out))
}

func serveLogFile(c *gin.Context, source, path, format string, watcherID uint) {
	n, err := requestedLogLines(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	traceID := c.Query("trace_id")
	var content []string
	if traceID != "" {
		parsed, parseErr := uuid.Parse(traceID)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "trace_id must be a UUID"})
			return
		}
		traceID = parsed.String()
		content, err = tailTraceFile(c.Request.Context(), path, traceID, n)
	} else {
		content, err = tailFile(path, n)
	}
	missing := os.IsNotExist(err)
	if err != nil && !missing {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("read log file: %v", err)})
		return
	}
	if content == nil {
		content = []string{}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, LogFileResponse{Source: source, WatcherID: watcherID, LogFile: path, Format: format, Missing: missing, Lines: content, TraceID: traceID})
}

func (h *Handler) WatcherLogs(c *gin.Context) {
	watcher, err := h.findWatcher(c)
	if err != nil {
		return
	}
	serveLogFile(c, "watcher", agent.WatcherLogPath(h.logDir, watcher.ID), "json", watcher.ID)
}
