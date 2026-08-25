package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/database"
)

func (h *Handler) cleanupWatcherServices(ctx context.Context, watcher *database.Watcher) error {
	if watcher == nil || runtime.GOOS != "windows" {
		return nil
	}

	for _, svc := range watcher.Services {
		if err := h.cleanupServiceRuntime(ctx, &svc); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) removeWatcherInstallDir(watcher *database.Watcher) error {
	if watcher == nil {
		return nil
	}

	installDir := strings.TrimSpace(watcher.InstallDir)
	if installDir == "" {
		return nil
	}

	info, err := os.Stat(installDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to inspect watcher install dir %s: %w", installDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("watcher install path is not a directory: %s", installDir)
	}

	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("failed to delete watcher install dir %s: %w", installDir, err)
	}
	return nil
}

func (h *Handler) removeNSSMService(ctx context.Context, name string) error {
	run := h.runNSSMCommand
	if run == nil {
		run = func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, h.nssmPath, args...).CombinedOutput()
		}
	}
	out, err := run(ctx, "remove", name, "confirm")
	if err == nil || isServiceMissingOutput(string(out)) {
		return nil
	}
	return fmt.Errorf("failed to remove service %s before watcher deletion: %s", name, strings.TrimSpace(string(out)))
}

func isServiceMissingOutput(out string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(out))
	return strings.Contains(normalized, "DOES NOT EXIST") ||
		strings.Contains(normalized, "SERVICE_DOES_NOT_EXIST") ||
		strings.Contains(normalized, "OPENSERVICE(): THE SPECIFIED SERVICE DOES NOT EXIST")
}

func (h *Handler) cleanupServiceRuntime(ctx context.Context, svc *database.Service) error {
	if svc == nil || !h.runningOnWindows() {
		return nil
	}
	if normalizeServiceType(svc.ServiceType) != "nssm" {
		return nil
	}

	name := strings.TrimSpace(svc.WindowsServiceName)
	if name == "" {
		return nil
	}

	if err := h.serviceManager.Stop(ctx, name); err != nil && !errors.Is(err, agent.ErrServiceNotFound) {
		return fmt.Errorf("failed to stop service %s before deletion: %w", name, err)
	}
	if err := h.removeNSSMService(ctx, name); err != nil {
		return err
	}
	return nil
}

// ── Service CRUD (nested under watcher) ───────────────────────

// GetServiceDetail returns a single service with parent watcher info (flat route).
