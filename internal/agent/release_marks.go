package agent

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// releaseNotesLimit caps stored notes; GitHub accepts bodies far larger than a review needs.
const releaseNotesLimit = 64 << 10

// ErrReleaseBlocked matches every attempt to deploy a version an operator blocked.
var ErrReleaseBlocked = errors.New("release is blocked")

// ReleaseBlockedError names the blocked version and why it was blocked.
type ReleaseBlockedError struct {
	Version string
	Reason  string
}

func (e *ReleaseBlockedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("release %s is blocked", e.Version)
	}
	return fmt.Sprintf("release %s is blocked: %s", e.Version, e.Reason)
}

func (e *ReleaseBlockedError) Is(target error) bool { return target == ErrReleaseBlocked }

// ReleaseNotes is what the release source publishes about a version.
type ReleaseNotes struct {
	Tag         string
	Title       string
	Notes       string
	URL         string
	PublishedAt *time.Time
}

// NotesFromGitHubRelease captures a release's description from an already-fetched release object.
func NotesFromGitHubRelease(release *CatalogRelease) *ReleaseNotes {
	if release == nil {
		return nil
	}
	notes := &ReleaseNotes{
		Tag:   strings.TrimSpace(release.TagName),
		Title: strings.TrimSpace(release.Name),
		Notes: truncateReleaseNotes(release.Body),
		URL:   safeReleaseURL(release.HTMLURL),
	}
	if published, err := time.Parse(time.RFC3339, release.PublishedAt); err == nil {
		published = published.UTC()
		notes.PublishedAt = &published
	}
	return notes
}

// safeReleaseURL keeps only absolute http(s) links, so a source cannot put a script URL in the dashboard.
func safeReleaseURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return u.String()
}

func truncateReleaseNotes(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= releaseNotesLimit {
		return body
	}
	return strings.ToValidUTF8(body[:releaseNotesLimit], "") + "\n\n…"
}

// RecordReleaseNotes stores a version's published notes without touching its block.
// Unchanged notes are not rewritten, so an up-to-date poll costs a single read.
func RecordReleaseNotes(db *gorm.DB, watcherID uint, version string, notes *ReleaseNotes) error {
	version = strings.TrimSpace(version)
	if notes == nil || version == "" {
		return nil
	}
	var existing database.ReleaseMark
	err := db.Where("watcher_id = ? AND version = ?", watcherID, version).Take(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && existing.Tag == notes.Tag && existing.Title == notes.Title && existing.Notes == notes.Notes &&
		existing.URL == notes.URL && sameTime(existing.PublishedAt, notes.PublishedAt) {
		return nil
	}
	mark := database.ReleaseMark{
		WatcherID:   watcherID,
		Version:     version,
		Tag:         notes.Tag,
		Title:       notes.Title,
		Notes:       notes.Notes,
		URL:         notes.URL,
		PublishedAt: notes.PublishedAt,
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "watcher_id"}, {Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"tag", "title", "notes", "url", "published_at", "updated_at"}),
	}).Create(&mark).Error
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// CheckReleaseAllowed returns a *ReleaseBlockedError when the watcher's version is blocked.
func CheckReleaseAllowed(db *gorm.DB, watcherID uint, version string) error {
	version = strings.TrimSpace(version)
	if version == "" {
		return nil
	}
	var mark database.ReleaseMark
	err := db.Select("block_reason").Where("watcher_id = ? AND version = ? AND blocked_at IS NOT NULL", watcherID, version).Take(&mark).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read release block for %s: %w", version, err)
	}
	return &ReleaseBlockedError{Version: version, Reason: mark.BlockReason}
}

// BlockRelease blocks a version, recording it even when Watcher has never seen it.
func BlockRelease(db *gorm.DB, watcherID uint, version, reason string) (*database.ReleaseMark, error) {
	now := time.Now().UTC()
	mark := database.ReleaseMark{WatcherID: watcherID, Version: version, BlockedAt: &now, BlockReason: reason}
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "watcher_id"}, {Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"blocked_at", "block_reason", "updated_at"}),
	}).Create(&mark).Error
	if err != nil {
		return nil, err
	}
	return &mark, db.Where("watcher_id = ? AND version = ?", watcherID, version).Take(&mark).Error
}

// UnblockRelease clears a version's block and keeps its notes.
func UnblockRelease(db *gorm.DB, watcherID uint, version string) (*database.ReleaseMark, error) {
	var mark database.ReleaseMark
	if err := db.Where("watcher_id = ? AND version = ?", watcherID, version).Take(&mark).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&mark).Updates(map[string]any{"blocked_at": nil, "block_reason": ""}).Error; err != nil {
		return nil, err
	}
	mark.BlockedAt, mark.BlockReason = nil, ""
	return &mark, nil
}

// releaseBlockedFunc reports blocks for the deployer, which has no database of its own.
// A lookup error is not treated as a block, so compensation can still restore a release.
func releaseBlockedFunc(db *gorm.DB, watcherID uint) func(string) bool {
	return func(version string) bool {
		return errors.Is(CheckReleaseAllowed(db, watcherID, version), ErrReleaseBlocked)
	}
}
