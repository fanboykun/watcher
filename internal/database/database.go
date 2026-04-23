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
	// Before/while migrating away from soft-delete semantics, purge historical
	// soft-deleted rows so names become reusable and old deleted rows don't reappear.
	if err := purgeLegacySoftDeletedRows(db); err != nil {
		return fmt.Errorf("purge legacy soft-deleted rows: %w", err)
	}

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
		{model: &Watcher{}, column: "release_ref", field: "ReleaseRef"},
		{model: &Watcher{}, column: "github_token", field: "GitHubToken"},
		{model: &DeployLog{}, column: "triggered_by", field: "TriggeredBy"},
		{model: &Service{}, column: "env_content", field: "EnvContent"},
		{model: &Service{}, column: "iis_app_kind", field: "IISAppKind"},
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

	if err := ensureWatcherServiceNameIsNonUnique(db); err != nil {
		return fmt.Errorf("ensure non-unique service_name index: %w", err)
	}

	if err := normalizeLegacyServiceTypes(db); err != nil {
		return fmt.Errorf("normalize legacy service types: %w", err)
	}

	return nil
}

func normalizeLegacyServiceTypes(db *gorm.DB) error {
	if err := db.Exec("UPDATE services SET service_type = 'iis' WHERE LOWER(TRIM(service_type)) = 'static'").Error; err != nil {
		return fmt.Errorf("rewrite legacy static service_type: %w", err)
	}

	if err := db.Exec(`
		UPDATE services
		SET iis_app_kind = CASE
			WHEN LOWER(TRIM(COALESCE(iis_managed_runtime, ''))) IN ('v2', 'v2.0', 'v4', 'v4.0', '.net clr v4.0') THEN 'aspnet_classic'
			ELSE 'static'
		END
		WHERE service_type = 'iis' AND (iis_app_kind IS NULL OR TRIM(iis_app_kind) = '')
	`).Error; err != nil {
		return fmt.Errorf("backfill iis_app_kind: %w", err)
	}

	return nil
}

func ensureWatcherServiceNameIsNonUnique(db *gorm.DB) error {
	const indexName = "idx_watchers_service_name"

	isUnique, err := indexIsUnique(db, "watchers", indexName)
	if err != nil {
		return err
	}
	if isUnique {
		if err := db.Migrator().DropIndex(&Watcher{}, indexName); err != nil {
			return fmt.Errorf("drop unique index %s: %w", indexName, err)
		}
	}
	if !db.Migrator().HasIndex(&Watcher{}, indexName) {
		if err := db.Migrator().CreateIndex(&Watcher{}, "ServiceName"); err != nil {
			return fmt.Errorf("create index %s: %w", indexName, err)
		}
	}

	if err := db.Exec("UPDATE watchers SET release_ref = 'latest' WHERE release_ref IS NULL OR TRIM(release_ref) = ''").Error; err != nil {
		return fmt.Errorf("backfill release_ref: %w", err)
	}

	return nil
}

func purgeLegacySoftDeletedRows(db *gorm.DB) error {
	// Child-first ordering keeps FK constraints happy on legacy schemas.
	tables := []string{
		"service_config_files",
		"services",
		"watchers",
	}

	for _, table := range tables {
		ok, err := tableHasColumn(db, table, "deleted_at")
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := db.Exec(fmt.Sprintf("DELETE FROM %s WHERE deleted_at IS NOT NULL", table)).Error; err != nil {
			return fmt.Errorf("delete soft-deleted rows in %s: %w", table, err)
		}
	}

	return nil
}

func tableHasColumn(db *gorm.DB, table, col string) (bool, error) {
	rows, err := db.Raw("PRAGMA table_info(" + table + ")").Rows()
	if err != nil {
		return false, fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan table_info(%s): %w", table, err)
		}
		if name == col {
			return true, nil
		}
	}

	return false, nil
}

func indexIsUnique(db *gorm.DB, table, indexName string) (bool, error) {
	rows, err := db.Raw("PRAGMA index_list(" + table + ")").Rows()
	if err != nil {
		return false, fmt.Errorf("pragma index_list(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return false, fmt.Errorf("scan index_list(%s): %w", table, err)
		}
		if name == indexName {
			return unique == 1, nil
		}
	}

	return false, nil
}
