package agent

import (
	"context"
	"testing"
	"time"

	"github.com/fanboykun/watcher/internal/database"
	"gorm.io/gorm"
)

func awaitAgentSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func TestAgentReconfigurationJoinsCompensationAndSettlesPrivateQueue(t *testing.T) {
	r, svc := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).Updates(map[string]any{"paused": true, "pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "approved"}).Error; err != nil {
		t.Fatal(err)
	}
	var watcher database.Watcher
	if err := r.db.First(&watcher, r.watcherID).Error; err != nil {
		t.Fatal(err)
	}
	attempt := queueCatalogTestAttempt(t, r, "v2")
	a := NewAgent(r.db, r.global, r.log, nil, nil, nil, nil)
	oldCtx, oldCancel := context.WithCancel(context.Background())
	oldDone, canceled, releaseRecovery := make(chan struct{}), make(chan struct{}), make(chan struct{})
	private := make(chan CheckTrigger, 1)
	private <- CheckTrigger{WatcherID: watcher.ID, CatalogID: 7, DeployLogID: attempt.ID}
	a.watchers[watcher.ID] = watcherHandle{cancel: oldCancel, trigger: private, done: oldDone, updatedAt: watcher.UpdatedAt.UnixNano() - 1}
	go func() {
		<-oldCtx.Done()
		close(canceled)
		<-releaseRecovery
		// Simulate the old worker finishing its non-cancelable rollback reconciliation.
		if err := r.db.Model(&svc).UpdateColumn("env_content", "VALUE=recovered").Error; err != nil {
			t.Error(err)
		}
		close(oldDone)
	}()
	reloadedServices := make(chan struct{}, 1)
	callback := "test:reloaded_services_after_compensation"
	if err := r.db.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table != "services" || tx.Error != nil {
			return
		}
		select {
		case <-oldDone:
			select {
			case reloadedServices <- struct{}{}:
			default:
			}
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer r.db.Callback().Query().Remove(callback)
	synced := make(chan struct{})
	go func() { a.syncWatchers(context.Background()); close(synced) }()
	awaitAgentSignal(t, canceled, "old worker cancellation")
	select {
	case <-synced:
		t.Fatal("replacement started before compensation finished")
	default:
	}
	close(releaseRecovery)
	awaitAgentSignal(t, synced, "worker replacement")
	awaitAgentSignal(t, reloadedServices, "post-compensation service reload")
	a.stopWatchers()
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := r.db.First(&svc, svc.ID).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.CompletedAt == nil || attempt.Status != "failed" || len(private) != 0 || svc.EnvContent != "VALUE=recovered" {
		t.Fatal("replacement lost old queue or recovery state")
	}
}

func TestAgentShutdownJoinsAndDrainsBothCatalogQueues(t *testing.T) {
	calls := installNSSMCommandMock(t)
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "approved"}).Error; err != nil {
		t.Fatal(err)
	}
	privateAttempt := queueCatalogTestAttempt(t, r, "v2")
	other := database.Watcher{Name: "global-queue", InstallDir: t.TempDir(), PendingCatalogID: 8, PendingVersion: "v3", ApprovedVersion: "v3", Status: "approved"}
	if err := r.db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	rOther := *r
	rOther.watcherID = other.ID
	globalAttempt := queueCatalogTestAttempt(t, &rOther, "v3")
	global, private := make(chan CheckTrigger, 1), make(chan CheckTrigger, 1)
	global <- CheckTrigger{WatcherID: other.ID, CatalogID: 8, DeployLogID: globalAttempt.ID}
	private <- CheckTrigger{WatcherID: r.watcherID, CatalogID: 7, DeployLogID: privateAttempt.ID}
	a := NewAgent(r.db, r.global, r.log, nil, global, nil, nil)
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	done, canceled, releaseRecovery := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.watchers[r.watcherID] = watcherHandle{cancel: cancelWorker, trigger: private, done: done}
	go func() { <-workerCtx.Done(); close(canceled); <-releaseRecovery; close(done) }()
	stopped := make(chan struct{})
	go func() { a.shutdown(); close(stopped) }()
	awaitAgentSignal(t, canceled, "shutdown cancellation")
	select {
	case <-stopped:
		t.Fatal("shutdown returned before compensation finished")
	default:
	}
	close(releaseRecovery)
	awaitAgentSignal(t, stopped, "shutdown drain")
	for _, id := range []uint{privateAttempt.ID, globalAttempt.ID} {
		var attempt database.DeployLog
		if err := r.db.First(&attempt, id).Error; err != nil {
			t.Fatal(err)
		}
		if attempt.CompletedAt == nil || attempt.Status != "failed" {
			t.Fatalf("shutdown left attempt %d open", id)
		}
	}
	if len(*calls) != 0 || len(global) != 0 || len(private) != 0 {
		t.Fatal("shutdown executed queued work or did not drain it")
	}
	close(global)
	a.drainCatalogTriggers(global, "closed queue")
}

func TestCanceledCatalogRequestSettlesWithoutStartingServices(t *testing.T) {
	calls := installNSSMCommandMock(t)
	r, _ := candidateFixture(t)
	if err := r.db.Model(&database.Watcher{}).Where("id = ?", r.watcherID).UpdateColumns(map[string]any{"pending_catalog_id": 7, "pending_version": "v2", "approved_version": "v2", "status": "approved"}).Error; err != nil {
		t.Fatal(err)
	}
	attempt := queueCatalogTestAttempt(t, r, "v2")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.RunCatalog(ctx, 7, attempt.ID); err == nil {
		t.Fatal("canceled request succeeded")
	}
	if err := r.db.First(&attempt, attempt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 || attempt.Status != "failed" || attempt.CompletedAt == nil {
		t.Fatal("canceled request ran services or retained open attempt")
	}
}
