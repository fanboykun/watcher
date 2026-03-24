package config

import (
	"fmt"
	"os"
	"strings"
)

// UpdateEnvFile updates/creates key-value entries in a .env-style file.
// Existing comments and unknown keys are preserved.
func UpdateEnvFile(path string, updates map[string]string) error {
	if path == "" {
		return fmt.Errorf("env path is empty")
	}

	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read env file: %w", err)
	}

	text := string(content)
	lines := []string{}
	if text != "" {
		lines = strings.Split(text, "\n")
	}

	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:eq])
		val, ok := updates[key]
		if !ok {
			continue
		}
		lines[i] = fmt.Sprintf("%s=%s", key, val)
		seen[key] = true
	}

	if len(lines) == 0 {
		lines = make([]string, 0, len(updates))
	}

	orderedKeys := []string{
		"ENVIRONMENT",
		"GITHUB_TOKEN",
		"LOG_DIR",
		"NSSM_PATH",
		"DB_PATH",
		"API_PORT",
		"API_BASE_URL",
		"WATCHER_REPO_URL",
		"WATCHER_SERVICE_NAME",
	}
	for _, key := range orderedKeys {
		val, ok := updates[key]
		if !ok || seen[key] {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s=%s", key, val))
	}

	out := strings.Join(lines, "\n")
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if err := os.WriteFile(path, []byte(out), 0600); err != nil {
		return fmt.Errorf("write env file: %w", err)
	}
	return nil
}
