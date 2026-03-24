package database

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func TestNewDBAddsWatcherGitHubTokenColumnForLegacyDB(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	legacyDB, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	if err := legacyDB.Exec(`DROP TABLE watchers`).Error; err != nil {
		t.Fatalf("drop watchers: %v", err)
	}

	createLegacyWatchers := `
	CREATE TABLE watchers (
		id integer primary key autoincrement,
		name text not null,
		service_name text not null,
		metadata_url text not null,
		check_interval_sec integer not null default 300,
		download_retries integer not null default 3,
		install_dir text not null,
		paused numeric not null default false,
		max_kept_versions integer not null default 3,
		hc_enabled numeric not null default false,
		hc_url text not null default '',
		hc_retries integer not null default 10,
		hc_interval_sec integer not null default 3,
		hc_timeout_sec integer not null default 5,
		current_version text not null default '',
		max_ignored_version text not null default '',
		status text not null default 'unknown',
		last_checked datetime,
		last_deployed datetime,
		last_error text not null default '',
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`
	if err := legacyDB.Exec(createLegacyWatchers).Error; err != nil {
		t.Fatalf("create legacy watchers: %v", err)
	}

	reopened, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("reopen db with migrations: %v", err)
	}

	if !columnExists(t, reopened, "watchers", "github_token") {
		t.Fatalf("expected github_token column to be added")
	}
	if !columnExists(t, reopened, "watchers", "deployment_environment") {
		t.Fatalf("expected deployment_environment column to be added")
	}
}

func TestNewDBRenamesLegacyGitHubTokenColumn(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy_rename.db")

	legacyDB, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	if err := legacyDB.Exec(`DROP TABLE watchers`).Error; err != nil {
		t.Fatalf("drop watchers: %v", err)
	}

	createLegacyWatchers := `
	CREATE TABLE watchers (
		id integer primary key autoincrement,
		name text not null,
		service_name text not null,
		metadata_url text not null,
		deployment_environment text not null default '',
		git_hub_token text not null default '',
		check_interval_sec integer not null default 300,
		download_retries integer not null default 3,
		install_dir text not null,
		paused numeric not null default false,
		max_kept_versions integer not null default 3,
		hc_enabled numeric not null default false,
		hc_url text not null default '',
		hc_retries integer not null default 10,
		hc_interval_sec integer not null default 3,
		hc_timeout_sec integer not null default 5,
		current_version text not null default '',
		max_ignored_version text not null default '',
		status text not null default 'unknown',
		last_checked datetime,
		last_deployed datetime,
		last_error text not null default '',
		created_at datetime,
		updated_at datetime,
		deleted_at datetime
	)`
	if err := legacyDB.Exec(createLegacyWatchers).Error; err != nil {
		t.Fatalf("create legacy watchers: %v", err)
	}
	if err := legacyDB.Exec(`INSERT INTO watchers (name, service_name, metadata_url, deployment_environment, git_hub_token, check_interval_sec, download_retries, install_dir, paused, max_kept_versions, hc_enabled, hc_url, hc_retries, hc_interval_sec, hc_timeout_sec, current_version, max_ignored_version, status, last_error) VALUES ('name', 'svc', 'https://example.com', '', 'secret-token', 300, 3, 'C:\apps\watcher', 0, 3, 0, '', 10, 3, 5, '', '', 'unknown', '')`).Error; err != nil {
		t.Fatalf("insert legacy watcher: %v", err)
	}

	reopened, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("reopen db with migrations: %v", err)
	}

	if !columnExists(t, reopened, "watchers", "github_token") {
		t.Fatalf("expected github_token column to exist")
	}

	var token string
	if err := reopened.Raw("SELECT github_token FROM watchers WHERE service_name = ?", "svc").Scan(&token).Error; err != nil {
		t.Fatalf("select migrated github_token: %v", err)
	}
	if token != "secret-token" {
		t.Fatalf("expected github_token to be backfilled, got %q", token)
	}
}

func TestNewDBPurgesLegacySoftDeletedRows(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy_soft_delete.db")

	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	// Simulate legacy soft-delete columns and a soft-deleted watcher row.
	if err := db.Exec(`ALTER TABLE watchers ADD COLUMN deleted_at datetime`).Error; err != nil {
		t.Fatalf("add watchers.deleted_at: %v", err)
	}
	if err := db.Exec(`ALTER TABLE services ADD COLUMN deleted_at datetime`).Error; err != nil {
		t.Fatalf("add services.deleted_at: %v", err)
	}
	if err := db.Exec(`ALTER TABLE service_config_files ADD COLUMN deleted_at datetime`).Error; err != nil {
		t.Fatalf("add service_config_files.deleted_at: %v", err)
	}

	if err := db.Exec(`INSERT INTO watchers (name, service_name, metadata_url, deployment_environment, github_token, check_interval_sec, download_retries, install_dir, paused, max_kept_versions, hc_enabled, hc_url, hc_retries, hc_interval_sec, hc_timeout_sec, current_version, max_ignored_version, status, last_error, deleted_at) VALUES ('legacy', 'dup-name', 'https://example.com', '', '', 300, 3, 'C:\apps\watcher', 0, 3, 0, '', 10, 3, 5, '', '', 'unknown', '', CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatalf("insert soft-deleted watcher: %v", err)
	}

	if _, err := NewDB(dbPath); err != nil {
		t.Fatalf("reopen db with migration: %v", err)
	}

	reopened, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("reopen db second pass: %v", err)
	}

	// Should be able to reuse service_name immediately because soft-deleted row is purged.
	w := &Watcher{
		Name:             "new",
		ServiceName:      "dup-name",
		MetadataURL:      "https://example.com",
		InstallDir:       "C:\\apps\\watcher",
		CheckIntervalSec: 300,
		DownloadRetries:  3,
		MaxKeptVersions:  3,
	}
	if err := reopened.Create(w).Error; err != nil {
		t.Fatalf("expected duplicate service_name to be reusable, create failed: %v", err)
	}
}

func columnExists(t *testing.T, db *gorm.DB, table, column string) bool {
	t.Helper()

	rows, err := db.Raw("PRAGMA table_info(" + table + ")").Rows()
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
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
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == column {
			return true
		}
	}

	return false
}
