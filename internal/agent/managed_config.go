package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeReleaseConfigFiles writes configuration whose target is the staged release.
func (d *Deployer) writeReleaseConfigFiles(currentDir string) error {
	for _, svc := range d.wcfg.Services {
		for _, file := range svc.ConfigFiles {
			if strings.TrimSpace(file.FilePath) == "" || normalizeConfigFileTarget(file.Target) != "release_dir" {
				continue
			}
			if err := writeManagedFile(currentDir, file.FilePath, file.Content); err != nil {
				return fmt.Errorf("write release config %s for %s: %w", file.FilePath, svc.WindowsServiceName, err)
			}
		}
	}
	return nil
}

// normalizeConfigFileTarget normalizes a configured file target to a supported location.
func normalizeConfigFileTarget(target string) string {
	switch strings.TrimSpace(strings.ToLower(target)) {
	case "", "app", "app_dir", "install_dir":
		return "app_dir"
	case "release", "release_dir", "current":
		return "release_dir"
	default:
		return "app_dir"
	}
}

// writeManagedFile writes a managed configuration file without allowing root traversal.
func writeManagedFile(rootDir, relativePath, content string) error {
	targetPath, err := managedFilePath(rootDir, relativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	return os.WriteFile(targetPath, []byte(content), 0600)
}

// managedFilePath resolves and validates a managed file path beneath its configured root.
func managedFilePath(rootDir, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", errors.New("managed file path is empty")
	}
	root, err := filepath.Abs(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve managed file root: %w", err)
	}
	target, err := filepath.Abs(filepath.Join(root, relativePath))
	if err != nil {
		return "", fmt.Errorf("resolve managed file path %q: %w", relativePath, err)
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("managed file path %q escapes root %s", relativePath, rootDir)
	}
	return target, nil
}
