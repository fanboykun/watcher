package database

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// NewDB opens (or creates) a SQLite database at the given path,
// configures it for concurrent use, and runs AutoMigrate.
func NewDB(dbPath string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", dbPath)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.AutoMigrate(
		&Watcher{},
		&Service{},
		&ServiceConfigFile{},
		&DeployLog{},
		&HealthEvent{},
		&PollEvent{},
	); err != nil {
		return nil, fmt.Errorf("failed to auto migrate tables: %w", err)
	}

	if err := ensureSchemaCompatibility(db); err != nil {
		return nil, fmt.Errorf("failed to apply compatibility migrations: %w", err)
	}

	return db, nil
}

func ensureSchemaCompatibility(db *gorm.DB) error {
	if db.Migrator().HasColumn(&Watcher{}, "git_hub_token") {
		if !db.Migrator().HasColumn(&Watcher{}, "github_token") {
			if err := db.Migrator().AddColumn(&Watcher{}, "GitHubToken"); err != nil {
				return fmt.Errorf("add corrected github_token column: %w", err)
			}
		}
		if err := db.Exec("UPDATE watchers SET github_token = git_hub_token WHERE github_token IS NULL OR github_token = ''").Error; err != nil {
			return fmt.Errorf("backfill github_token from git_hub_token: %w", err)
		}
	}

	type columnSpec struct {
		model  any
		column string
		field  string
	}

	columns := []columnSpec{
		{model: &Watcher{}, column: "deployment_environment", field: "DeploymentEnvironment"},
		{model: &Watcher{}, column: "github_token", field: "GitHubToken"},
		{model: &DeployLog{}, column: "triggered_by", field: "TriggeredBy"},
		{model: &Service{}, column: "env_content", field: "EnvContent"},
	}

	for _, spec := range columns {
		if db.Migrator().HasColumn(spec.model, spec.column) {
			continue
		}
		if err := db.Migrator().AddColumn(spec.model, spec.field); err != nil {
			return fmt.Errorf("add column %s: %w", spec.column, err)
		}
	}

	if !db.Migrator().HasTable(&ServiceConfigFile{}) {
		if err := db.Migrator().CreateTable(&ServiceConfigFile{}); err != nil {
			return fmt.Errorf("create table service_config_files: %w", err)
		}
	}

	return nil
}
