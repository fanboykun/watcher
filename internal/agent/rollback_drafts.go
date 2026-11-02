package agent

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

// SnapshotSourceRollbackEdit marks a snapshot rewritten by a rollback that applied saved drafts.
const SnapshotSourceRollbackEdit = "rollback_edit"

// RollbackConfigService pairs a service's saved configuration for a version with the
// draft that a manual rollback to that version would apply on top of it.
type RollbackConfigService struct {
	ServiceID     uint                           `json:"service_id"`
	ServiceName   string                         `json:"service_name"`
	EnvFile       string                         `json:"env_file"`
	SnapshotEnv   string                         `json:"snapshot_env"`
	SnapshotFiles []database.RevisionConfigFile  `json:"snapshot_files"`
	HasDraft      bool                           `json:"has_draft"`
	DraftEnv      string                         `json:"draft_env"`
	DraftFiles    *[]database.RevisionConfigFile `json:"draft_files"`
}

// RollbackConfig is the editable view of one retained version's configuration.
type RollbackConfig struct {
	Version    string                  `json:"version"`
	Source     string                  `json:"source"`
	CapturedAt time.Time               `json:"captured_at"`
	Services   []RollbackConfigService `json:"services"`
}

// ReadRollbackConfig projects a version's snapshot onto the configured services and
// attaches the drafts saved for exactly that version. "next" drafts never apply to rollback.
func ReadRollbackConfig(db *gorm.DB, watcherID uint, version string) (*RollbackConfig, error) {
	wcfg, services, revisions, err := loadRollbackDrafts(db, watcherID, version)
	if err != nil {
		return nil, err
	}
	manifest, err := loadConfigSnapshotManifest(wcfg.InstallDir, version)
	if err != nil {
		return nil, err
	}
	result := &RollbackConfig{Version: version, Source: manifest.Source, CapturedAt: manifest.CapturedAt, Services: []RollbackConfigService{}}
	for _, svc := range services {
		item := RollbackConfigService{
			ServiceID:     svc.ID,
			ServiceName:   serviceDisplayName(svc),
			EnvFile:       svc.EnvFile,
			SnapshotEnv:   svc.EnvContent,
			SnapshotFiles: revisionFiles(svc.ConfigFiles),
		}
		if rev, ok := revisions[svc.ID]; ok {
			item.HasDraft, item.DraftEnv, item.DraftFiles = true, rev.EnvContent, rev.ConfigFiles
		}
		result.Services = append(result.Services, item)
	}
	return result, nil
}

// StageRollbackDrafts rewrites the version's snapshot as that snapshot plus its saved drafts,
// so the ordinary rollback restores the edited configuration. finish must be called with the
// rollback result: on failure it puts the original snapshot back, and on success it consumes
// the drafts that were applied. Automatic rollback never stages drafts.
func StageRollbackDrafts(db *gorm.DB, watcherID uint, version string) (int, func(error) error, error) {
	wcfg, services, revisions, err := loadRollbackDrafts(db, watcherID, version)
	if err != nil {
		return 0, nil, err
	}
	if len(revisions) == 0 {
		return 0, func(cause error) error { return cause }, nil
	}
	applied := make([]database.ServiceConfigRevision, 0, len(revisions))
	for i := range services {
		rev, ok := revisions[services[i].ID]
		if !ok {
			continue
		}
		if strings.TrimSpace(services[i].EnvFile) != "" {
			services[i].EnvContent = rev.EnvContent
		}
		if rev.ConfigFiles != nil {
			services[i].ConfigFiles = make([]ConfigFile, 0, len(*rev.ConfigFiles))
			for _, file := range *rev.ConfigFiles {
				services[i].ConfigFiles = append(services[i].ConfigFiles, ConfigFile{FilePath: file.FilePath, Target: file.Target, Content: file.Content})
			}
		}
		applied = append(applied, rev)
	}
	if err := validateDeploymentManagedFiles(services); err != nil {
		return 0, nil, fmt.Errorf("validate draft configuration for %s: %w", version, err)
	}

	restore, err := preserveConfigSnapshot(wcfg.InstallDir, version)
	if err != nil {
		return 0, nil, err
	}
	staged := *wcfg
	staged.Services = services
	if err := CaptureConfigSnapshot(&staged, version, SnapshotSourceRollbackEdit); err != nil {
		return 0, nil, restore(fmt.Errorf("stage draft configuration for %s: %w", version, err))
	}
	finish := func(cause error) error {
		if cause != nil {
			return restore(cause)
		}
		_ = restore(nil)
		return consumeRollbackDrafts(db, applied)
	}
	return len(applied), finish, nil
}

// RollbackDraftVersions lists the versions with drafts that a manual rollback would apply.
func RollbackDraftVersions(db *gorm.DB, watcherID uint) ([]string, error) {
	versions := []string{}
	err := db.Model(&database.ServiceConfigRevision{}).
		Where("target_version <> ? AND service_id IN (?)", "next", db.Model(&database.Service{}).Select("id").Where("watcher_id = ?", watcherID)).
		Distinct("target_version").Order("target_version ASC").Pluck("target_version", &versions).Error
	return versions, err
}

func loadRollbackDrafts(db *gorm.DB, watcherID uint, version string) (*WatcherConfig, []ServiceConfig, map[uint]database.ServiceConfigRevision, error) {
	var watcher database.Watcher
	if err := db.
		Preload("Services", func(tx *gorm.DB) *gorm.DB { return tx.Order("id ASC") }).
		Preload("Services.ConfigFiles", func(tx *gorm.DB) *gorm.DB { return tx.Order("id ASC") }).
		First(&watcher, watcherID).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load watcher config: %w", err)
	}
	wcfg := WatcherConfigFromDB(&watcher)
	services, err := readConfigSnapshotServices(wcfg, version)
	if err != nil {
		return nil, nil, nil, err
	}
	ids := make([]uint, 0, len(watcher.Services))
	for _, svc := range watcher.Services {
		ids = append(ids, svc.ID)
	}
	revisions := map[uint]database.ServiceConfigRevision{}
	if len(ids) == 0 {
		return wcfg, services, revisions, nil
	}
	var rows []database.ServiceConfigRevision
	if err := db.Where("service_id IN ? AND target_version = ?", ids, version).Find(&rows).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load drafts for %s: %w", version, err)
	}
	for _, row := range rows {
		revisions[row.ServiceID] = row
	}
	return wcfg, services, revisions, nil
}

// consumeRollbackDrafts deletes applied drafts that were not edited while the rollback ran.
func consumeRollbackDrafts(db *gorm.DB, applied []database.ServiceConfigRevision) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, revision := range applied {
			var current database.ServiceConfigRevision
			err := tx.First(&current, revision.ID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if current.UpdatedAt.Equal(revision.UpdatedAt) && current.EnvContent == revision.EnvContent && reflect.DeepEqual(current.ConfigFiles, revision.ConfigFiles) {
				if err := tx.Delete(&current).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func revisionFiles(files []ConfigFile) []database.RevisionConfigFile {
	out := make([]database.RevisionConfigFile, 0, len(files))
	for _, file := range files {
		out = append(out, database.RevisionConfigFile{FilePath: file.FilePath, Target: file.Target, Content: file.Content})
	}
	return out
}

func serviceDisplayName(svc ServiceConfig) string {
	for _, name := range []string{svc.WindowsServiceName, svc.IISSiteName, svc.IISAppPool, svc.BinaryName} {
		if strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return fmt.Sprintf("Service %d", svc.ID)
}
