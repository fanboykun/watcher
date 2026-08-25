package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const legacyConfigSnapshotStoreDir = "legacy-snapshots"

// PrepareConfigSnapshots captures only the active version and quarantines legacy snapshots.
func PrepareConfigSnapshots(wcfg *WatcherConfig, currentVersion string, log *Logger) {
	currentVersion = strings.TrimSpace(currentVersion)
	if currentVersion != "" && !HasConfigSnapshot(wcfg.InstallDir, currentVersion) {
		if err := CaptureConfigSnapshot(wcfg, currentVersion, SnapshotSourceMigration); err != nil {
			log.Error("snapshot migration: failed to capture current version", "watcher", wcfg.Name, "version", currentVersion, "error", err)
			return
		}
		log.Info("captured trusted config snapshot for current version", "watcher", wcfg.Name, "version", currentVersion)
	}
	quarantineLegacyConfigSnapshots(wcfg.InstallDir, log, wcfg.Name)
}

func quarantineLegacyConfigSnapshots(installDir string, log *Logger, watcherName string) {
	versions, err := ListAvailableVersions(installDir)
	if err != nil {
		log.Warn("snapshot migration: failed to list versions", "watcher", watcherName, "error", err)
		return
	}
	legacyRoot := filepath.Join(installDir, watcherStateDir, legacyConfigSnapshotStoreDir)
	for _, version := range versions {
		source := filepath.Join(version.Path, legacyConfigSnapshotDir)
		if info, err := os.Stat(source); err != nil || !info.IsDir() {
			continue
		}
		if err := os.MkdirAll(legacyRoot, 0700); err != nil {
			log.Warn("snapshot migration: failed to create legacy store", "watcher", watcherName, "error", err)
			return
		}
		target := filepath.Join(legacyRoot, releaseStorageName(version.Version))
		if _, err := os.Lstat(target); err == nil {
			target = fmt.Sprintf("%s-%d", target, time.Now().UnixNano())
		} else if !os.IsNotExist(err) {
			log.Warn("snapshot migration: failed to inspect legacy destination", "watcher", watcherName, "version", version.Version, "error", err)
			continue
		}
		if err := os.Rename(source, target); err != nil {
			log.Warn("snapshot migration: failed to quarantine legacy snapshot", "watcher", watcherName, "version", version.Version, "error", err)
			continue
		}
		log.Info("quarantined untrusted legacy config snapshot", "watcher", watcherName, "version", version.Version, "path", target)
	}
}
