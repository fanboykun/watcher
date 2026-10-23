package agent

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

// prepareCandidateServices resolves drafts in memory. It never modifies active
// persistence or live files; the deployer applies files after service shutdown.
func (r *RepoWatcher) prepareCandidateServices(version string) ([]database.ServiceConfigRevision, error) {
	services := append([]ServiceConfig(nil), r.wcfg.Services...)
	var selected []database.ServiceConfigRevision
	for i := range services {
		if services[i].ID == 0 {
			continue
		}
		var revision database.ServiceConfigRevision
		err := r.db.Where("service_id = ? AND target_version = ?", services[i].ID, version).First(&revision).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && version != "next" {
			err = r.db.Where("service_id = ? AND target_version = ?", services[i].ID, "next").First(&revision).Error
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("load candidate for service %d: %w", services[i].ID, err)
		}
		services[i].EnvContent = revision.EnvContent
		if revision.ConfigFiles != nil {
			services[i].ConfigFiles = make([]ConfigFile, 0, len(*revision.ConfigFiles))
			for _, file := range *revision.ConfigFiles {
				services[i].ConfigFiles = append(services[i].ConfigFiles, ConfigFile{FilePath: file.FilePath, Target: file.Target, Content: file.Content})
			}
		}
		selected = append(selected, revision)
	}
	if err := validateDeploymentManagedFiles(services); err != nil {
		return nil, err
	}
	r.deployer.previousServices = append([]ServiceConfig(nil), r.wcfg.Services...)
	r.wcfg.Services = services
	return selected, nil
}

// commitCandidateServices consumes only the revisions actually deployed, after
// health verification. Edits made during deployment remain available as drafts.
func (r *RepoWatcher) commitCandidateServices(selected []database.ServiceConfigRevision) error {
	if len(selected) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, revision := range selected {
			if err := tx.Model(&database.Service{}).Where("id = ? AND watcher_id = ?", revision.ServiceID, r.watcherID).
				Update("env_content", revision.EnvContent).Error; err != nil {
				return err
			}
			if revision.ConfigFiles != nil {
				if err := replaceServiceManagedFiles(tx, revision.ServiceID, *revision.ConfigFiles); err != nil {
					return err
				}
			}
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

func replaceServiceManagedFiles(tx *gorm.DB, serviceID uint, files []database.RevisionConfigFile) error {
	if err := tx.Where("service_id = ?", serviceID).Delete(&database.ServiceConfigFile{}).Error; err != nil {
		return err
	}
	for _, file := range files {
		if err := tx.Create(&database.ServiceConfigFile{ServiceID: serviceID, FilePath: file.FilePath, Target: file.Target, Content: file.Content}).Error; err != nil {
			return err
		}
	}
	return nil
}
