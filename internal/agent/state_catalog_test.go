package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
)

func TestCatalogFailureSettlementIsAtomicAndPublishesAfterCommit(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.AutoMigrate(&database.WebhookEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{
		"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "approved",
		"webhook_enabled": true, "webhook_url": "https://example.com/hook", "webhook_signing_secret": "secret", "notify_deployment_failed": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	attempt := queueCatalogTestAttempt(t, r, "v2")
	bus := NewWatcherEventBus()
	events, unsubscribe := bus.Subscribe(r.watcherID)
	defer unsubscribe()
	state := NewStateManager(r.db, r.watcherID, r.log, bus, webhook.NewService(r.db, r.global, nil))
	state.catalogID, state.deployLogID = 7, attempt.ID
	if err := r.db.Exec(`CREATE TRIGGER fail_catalog_log BEFORE UPDATE ON deploy_logs BEGIN SELECT RAISE(ABORT, 'injected log failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := state.SetFailedWithPhase("invalid local ZIP", "prepare"); err == nil {
		t.Fatal("log write failure was ignored")
	}
	var watcher database.Watcher
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.Status != "approved" || watcher.ApprovedVersion != "v2" || attempt.CompletedAt != nil {
		t.Fatalf("partial failure settlement: watcher=%+v attempt=%+v", watcher, attempt)
	}
	select {
	case event := <-events:
		t.Fatalf("event published before transaction commit: %+v", event)
	default:
	}
	var count int64
	if err := r.db.Model(&database.WebhookEvent{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("premature webhook count=%d error=%v", count, err)
	}
	if err := r.db.Exec(`DROP TRIGGER fail_catalog_log`).Error; err != nil {
		t.Fatal(err)
	}
	if err := state.SetFailedWithPhase("invalid local ZIP", "prepare"); err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if watcher.ApprovedVersion != "" || watcher.PendingCatalogID != 7 || attempt.CompletedAt == nil || attempt.FailurePhase != "prepare" {
		t.Fatal("committed failure did not close exact attempt and retain review candidate")
	}
	for _, want := range []string{EventDeployFinished, EventStatusChanged} {
		select {
		case event := <-events:
			if event.Type != want {
				t.Fatalf("event=%s want=%s", event.Type, want)
			}
		default:
			t.Fatalf("missing event %s", want)
		}
	}
	if err := r.db.Model(&database.WebhookEvent{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("committed webhook count=%d error=%v", count, err)
	}
}

func TestCatalogClosedAttemptCannotExecuteOrRejectSameArtifactRetry(t *testing.T) {
	calls := installNSSMCommandMock(t)
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "approved"}).Error; err != nil {
		t.Fatal(err)
	}
	old := queueCatalogTestAttempt(t, r, "v2")
	state := NewStateManager(r.db, r.watcherID, r.log, nil, nil)
	state.catalogID, state.deployLogID = 7, old.ID
	if err := state.SetFailed("first attempt failed"); err != nil {
		t.Fatal(err)
	}
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"approved_version": "v2", "status": "approved"}).Error; err != nil {
		t.Fatal(err)
	}
	retry := queueCatalogTestAttempt(t, r, "v2")
	a := NewAgent(r.db, r.global, r.log, nil, nil, nil, nil)
	a.rejectCatalogTrigger(CheckTrigger{WatcherID: r.watcherID, CatalogID: 7, DeployLogID: old.ID}, "late queue rejection")
	if err := r.RunCatalog(context.Background(), 7, old.ID); err == nil {
		t.Fatal("closed old attempt executed a retry")
	}
	if _, err := state.SetDeploying("v2", "v1"); err == nil {
		t.Fatal("closed old attempt acquired retry approval")
	}
	var watcher database.Watcher
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&retry, retry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 || watcher.Status != "approved" || watcher.ApprovedVersion != "v2" || retry.CompletedAt != nil {
		t.Fatal("stale same-artifact trigger changed services or retry ownership")
	}
}

func TestCatalogRecoveryClosesLegacyPartialFailure(t *testing.T) {
	for _, status := range []string{"failed", "healthy"} {
		t.Run(status, func(t *testing.T) {
			r, _ := candidateFixture(t)
			if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "", "status": status}).Error; err != nil {
				t.Fatal(err)
			}
			attempt := queueCatalogTestAttempt(t, r, "v2")
			a := NewAgent(r.db, r.global, r.log, nil, nil, nil, nil)
			a.recoverCatalogDeployments()
			if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
				t.Fatal(err)
			}
			var watcher database.Watcher
			if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
				t.Fatal(err)
			}
			if attempt.CompletedAt == nil || attempt.Status != "failed" || watcher.PendingCatalogID != 7 || watcher.ApprovedVersion != "" {
				t.Fatal("legacy partial failure remains stuck")
			}
		})
	}
}

func TestCatalogRecoverySettlesInterruptedLinkedRollback(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "deploying"}).Error; err != nil {
		t.Fatal(err)
	}
	primary := queueCatalogTestAttempt(t, r, "v2")
	state := NewStateManager(r.db, r.watcherID, r.log, nil, nil)
	state.catalogID, state.deployLogID = 7, primary.ID
	rollbackID, err := state.StartRollbackAttempt("v1", "v2", "v2", "auto_after_deploy_failure", "agent", &primary.ID, &primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SetFailed("health failed"); err != nil {
		t.Fatal(err)
	}
	a := NewAgent(r.db, r.global, r.log, nil, nil, nil, nil)
	a.recoverCatalogDeployments()
	var rollback database.DeployLog
	if err := r.db.First(&rollback, rollbackID).Error; err != nil {
		t.Fatal(err)
	}
	if rollback.Status != "failed" || rollback.CompletedAt == nil {
		t.Fatal("restart left linked rollback active")
	}
}

func TestCatalogRecoveryReleasesApprovalAfterLegacyAttemptAlreadyClosed(t *testing.T) {
	for _, status := range []string{"failed", "healthy"} {
		t.Run(status, func(t *testing.T) {
			r, _ := candidateFixture(t)
			if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{
				"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": status,
			}).Error; err != nil {
				t.Fatal(err)
			}
			attempt := queueCatalogTestAttempt(t, r, "v2")
			now := time.Now().UTC()
			if err := r.db.Model(&attempt).Updates(map[string]any{"status": "failed", "completed_at": now, "error": "original health failure"}).Error; err != nil {
				t.Fatal(err)
			}
			bus := NewWatcherEventBus()
			events, unsubscribe := bus.Subscribe(r.watcherID)
			defer unsubscribe()
			a := NewAgent(r.db, r.global, r.log, bus, nil, nil, nil)
			a.recoverCatalogDeployments()
			var watcher database.Watcher
			if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
				t.Fatal(err)
			}
			if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
				t.Fatal(err)
			}
			if watcher.Status != "failed" || watcher.ApprovedVersion != "" || watcher.PendingCatalogID != 7 || watcher.PendingVersion != "v2" || attempt.Error != "original health failure" {
				t.Fatal("legacy completed attempt retained unusable approval or lost its failure")
			}
			select {
			case event := <-events:
				if event.Type != EventStatusChanged {
					t.Fatalf("unexpected recovery event: %+v", event)
				}
			default:
				t.Fatal("approval recovery did not refresh the watcher")
			}
		})
	}
}

func TestCatalogRollbackIsOpenBeforePrimaryFailurePublishes(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "deploying"}).Error; err != nil {
		t.Fatal(err)
	}
	primary := queueCatalogTestAttempt(t, r, "v2")
	r.state.catalogID, r.state.deployLogID = 7, primary.ID
	if err := CaptureConfigSnapshot(r.wcfg, "v1", SnapshotSourceDeployment); err != nil {
		t.Fatal(err)
	}
	// A DB trigger observes the exact point where the primary log closes. Every
	// admissible retry sees an open rollback until final watcher state commits.
	if err := r.db.Exec(`CREATE TRIGGER require_open_catalog_rollback BEFORE UPDATE ON deploy_logs WHEN NEW.id = ` + fmt.Sprint(primary.ID) + ` AND NEW.status = 'failed' BEGIN SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM deploy_logs WHERE kind = 'rollback' AND parent_attempt_id = NEW.id AND completed_at IS NULL) THEN RAISE(ABORT, 'retry admission gap') END; END`).Error; err != nil {
		t.Fatal(err)
	}
	if !r.recordDeployFailure(&deployRecoveryError{cause: errors.New("health check failed"), rollbackVersion: "v1"}, "v2", "v1") {
		t.Fatal("rollback did not settle with continuous attempt guard")
	}
	var attempts []database.DeployLog
	if err := r.db.Where("watcher_id = ?", r.watcherID).Order("id").Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 2 || attempts[0].Status != "failed" || attempts[1].Status != "succeeded" || attempts[1].ParentAttemptID == nil || *attempts[1].ParentAttemptID != primary.ID {
		t.Fatalf("invalid recovery lineage: %+v", attempts)
	}
}

func TestCatalogRollbackCompletionIsAtomic(t *testing.T) {
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"status": "failed", "pending_catalog_id": 7, "pending_version": "v2"}).Error; err != nil {
		t.Fatal(err)
	}
	id, err := r.state.StartRollbackAttempt("v1", "v2", "v2", "auto_after_deploy_failure", "agent", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.db.Exec(`CREATE TRIGGER fail_rollback_watcher BEFORE UPDATE ON watchers WHEN NEW.status = 'healthy' BEGIN SELECT RAISE(ABORT, 'injected watcher failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.state.CompleteRollbackAttempt(id, "v1", ""); err == nil {
		t.Fatal("watcher failure ignored")
	}
	var attempt database.DeployLog
	if err := r.db.First(&attempt, id).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.CompletedAt != nil || attempt.Status != "in_progress" {
		t.Fatal("rollback admission guard released before watcher state committed")
	}
}
