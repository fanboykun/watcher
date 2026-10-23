package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type SnapshotConfigFile struct {
	FilePath string `json:"file_path"`
	Target   string `json:"target"`
	Content  string `json:"content"`
}

type SnapshotService struct {
	ServiceName string               `json:"service_name"`
	Files       []SnapshotConfigFile `json:"files"`
}

type ConfigSnapshot struct {
	Version    string            `json:"version"`
	Source     string            `json:"source"`
	CapturedAt time.Time         `json:"captured_at"`
	Services   []SnapshotService `json:"services"`
}

// ReadConfigSnapshot inspects the complete trusted tree, rather than projecting
// it through current service definitions. Historical paths and removed services
// must remain visible even after the active configuration changes.
func ReadConfigSnapshot(installDir, version string) (*ConfigSnapshot, error) {
	configSnapshotMu.Lock()
	defer configSnapshotMu.Unlock()

	manifest, err := loadConfigSnapshotManifest(installDir, version)
	if err != nil {
		return nil, err
	}
	result := &ConfigSnapshot{Version: manifest.Version, Source: manifest.Source, CapturedAt: manifest.CapturedAt, Services: []SnapshotService{}}
	root := filepath.Join(ConfigSnapshotPath(installDir, manifest.Version), "services")
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return result, nil // An empty configuration has no service trees.
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("snapshot services path is not a directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("snapshot contains symlink %s", entry.Name())
		}
		if !entry.IsDir() {
			continue
		}
		service := SnapshotService{ServiceName: entry.Name(), Files: []SnapshotConfigFile{}}
		for _, category := range []struct{ name, target string }{{"env", "env"}, {"app", "app_dir"}, {"release", "release_dir"}} {
			files, err := readSnapshotConfigFiles(filepath.Join(root, entry.Name(), category.name), category.target)
			if err != nil {
				return nil, fmt.Errorf("read snapshot %s/%s: %w", entry.Name(), category.name, err)
			}
			for _, file := range files {
				service.Files = append(service.Files, SnapshotConfigFile{FilePath: file.FilePath, Target: file.Target, Content: file.Content})
			}
		}
		result.Services = append(result.Services, service)
	}
	return result, nil
}
