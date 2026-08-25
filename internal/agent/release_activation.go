package agent

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// swapCurrent atomically activates a release with a junction and a safe copy fallback.
func (d *Deployer) swapCurrent(releaseDir, currentDir string) error {
	suffix := time.Now().UnixNano()
	candidateDir := fmt.Sprintf("%s.next-%d", currentDir, suffix)
	previousDir := fmt.Sprintf("%s.previous-%d", currentDir, suffix)
	defer removePath(candidateDir)

	out, err := runCommand("cmd", "/C", "mklink", "/J", candidateDir, releaseDir)
	if err != nil {
		d.lWarn("mklink /J failed, falling back to copy", "output", string(out))
		if err := removePath(candidateDir); err != nil {
			return fmt.Errorf("remove failed current candidate: %w", err)
		}
		if err := copyDir(releaseDir, candidateDir); err != nil {
			return fmt.Errorf("prepare current candidate: %w", err)
		}
	}

	hadCurrent := false
	if _, err := os.Lstat(currentDir); err == nil {
		hadCurrent = true
		if err := os.Rename(currentDir, previousDir); err != nil {
			return fmt.Errorf("preserve old current: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect current path: %w", err)
	}

	if err := os.Rename(candidateDir, currentDir); err != nil {
		failure := fmt.Errorf("activate current candidate: %w", err)
		if hadCurrent {
			if restoreErr := os.Rename(previousDir, currentDir); restoreErr != nil {
				failure = errors.Join(failure, fmt.Errorf("restore old current: %w", restoreErr))
			}
		}
		return failure
	}

	if hadCurrent {
		if err := removePath(previousDir); err != nil {
			d.lWarn("failed to remove previous current path", "path", previousDir, "error", err)
		}
	}
	return nil
}

// removePath removes path.
func removePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Remove(path); err == nil {
		return nil
	}
	return os.RemoveAll(path)
}

// ensureServiceByType dispatches to the correct ensure logic based on ServiceType.
