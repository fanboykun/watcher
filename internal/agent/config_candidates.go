package agent

import (
	"errors"
	"fmt"

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
		selected = append(selected, revision)
	}
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
			if err := tx.Where("id = ? AND updated_at = ? AND env_content = ?", revision.ID, revision.UpdatedAt, revision.EnvContent).
				Delete(&database.ServiceConfigRevision{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
