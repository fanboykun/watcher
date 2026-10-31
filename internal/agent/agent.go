package agent

import (
	"context"
	"sync"
	"time"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
	"gorm.io/gorm"
)

type watcherHandle struct {
	cancel    context.CancelFunc
	trigger   chan CheckTrigger
	updatedAt int64
	done      chan struct{}
}

// Agent manages all RepoWatcher instances, one goroutine per watcher entry.
type Agent struct {
	db           *gorm.DB
	appCfg       *config.AppConfig
	log          *Logger
	events       *WatcherEventBus
	checkTrigger chan CheckTrigger
	syncTrigger  chan struct{}
	webhooks     *webhook.Service

	mu       sync.Mutex
	watchers map[uint]watcherHandle
	polling  *PollingMonitor
	ready    chan struct{}
}

// NewAgent creates a configured agent.
func NewAgent(db *gorm.DB, appCfg *config.AppConfig, log *Logger, events *WatcherEventBus, checkTrigger chan CheckTrigger, syncTrigger chan struct{}, webhookService *webhook.Service, monitors ...*PollingMonitor) *Agent {
	monitor := NewPollingMonitor()
	if len(monitors) > 0 && monitors[0] != nil {
		monitor = monitors[0]
	}
	return &Agent{
		db:           db,
		appCfg:       appCfg,
		log:          log,
		events:       events,
		checkTrigger: checkTrigger,
		syncTrigger:  syncTrigger,
		webhooks:     webhookService,
		watchers:     make(map[uint]watcherHandle),
		polling:      monitor,
		ready:        make(chan struct{}),
	}
}

// Ready is closed once persisted recovery and initial worker reconciliation finish.
func (a *Agent) Ready() <-chan struct{} { return a.ready }

// Run starts the agent supervisor and blocks until its context is cancelled.
func (a *Agent) Run(ctx context.Context) {
	a.recoverCatalogDeployments()
	a.prepareConfigSnapshots()
	a.syncWatchers(ctx)
	close(a.ready)

	for {
		select {
		case <-ctx.Done():
			a.shutdown()
			a.log.Info("all watchers stopped")
			return
		case <-a.syncTrigger:
			a.log.Info("syncing watchers from database")
			a.syncWatchers(ctx)
		case trigger := <-a.checkTrigger:
			if ctx.Err() != nil {
				a.rejectCatalogTrigger(trigger, "catalog deployment interrupted by agent shutdown; review and retry")
				continue
			}
			a.mu.Lock()
			h, ok := a.watchers[trigger.WatcherID]
			a.mu.Unlock()
			if ok {
				select {
				case h.trigger <- trigger:
				default:
					a.log.WithTrace(trigger.Trace).Warn("poll trigger skipped: watcher queue is full", "watcher_id", trigger.WatcherID)
					a.rejectCatalogTrigger(trigger, "watcher queue is full")
				}
			} else {
				a.log.WithTrace(trigger.Trace).Warn("check trigger for unknown watcher", "watcher_id", trigger.WatcherID)
				a.rejectCatalogTrigger(trigger, "watcher runtime is unavailable")
			}
		}
	}
}

// prepareConfigSnapshots migrates legacy storage without inventing historical config.
func (a *Agent) prepareConfigSnapshots() {
	var watchers []database.Watcher
	if err := a.db.Preload("Services").Preload("Services.ConfigFiles").Find(&watchers).Error; err != nil {
		a.log.Error("backfill: failed to load watchers", "error", err)
		return
	}
	for i := range watchers {
		PrepareConfigSnapshots(WatcherConfigFromDB(&watchers[i]), watchers[i].CurrentVersion, a.log.WithWatcher(watchers[i].ID, watchers[i].Name))
	}
}

// syncWatchers reconciles running watcher goroutines with current database records.
func (a *Agent) syncWatchers(ctx context.Context) {
	var watchers []database.Watcher
	if err := a.db.Preload("Services").Preload("Services.ConfigFiles").Find(&watchers).Error; err != nil {
		a.log.Error("failed to load watchers from database", "error", err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	currentIDs := make(map[uint]bool)
	for _, w := range watchers {
		currentIDs[w.ID] = true
	}
	for id, h := range a.watchers {
		if !currentIDs[id] {
			a.log.Info("stopping removed watcher", "id", id)
			a.stopWatcher(h, "watcher was removed; review and retry")
			delete(a.watchers, id)
		}
	}

	for i := range watchers {
		w := &watchers[i]
		h, exists := a.watchers[w.ID]
		updatedAt := w.UpdatedAt.UnixNano()
		if exists && h.updatedAt == updatedAt {
			continue
		}
		if exists {
			a.log.Info("restarting updated watcher", "id", w.ID)
			a.stopWatcher(h, "watcher configuration changed; review and retry")
			// The previous worker can reconcile or commit configuration while joining.
			var latest database.Watcher
			if err := a.db.Preload("Services").Preload("Services.ConfigFiles").First(&latest, w.ID).Error; err != nil {
				a.log.Error("failed to reload watcher after stopping worker", "watcher_id", w.ID, "error", err)
				delete(a.watchers, w.ID)
				continue
			}
			w = &latest
			updatedAt = latest.UpdatedAt.UnixNano()
		} else {
			a.log.Info("starting new watcher", "id", w.ID)
		}
		if ctx.Err() != nil {
			delete(a.watchers, w.ID)
			continue
		}
		watcherCtx, cancel := context.WithCancel(ctx)
		trigger := make(chan CheckTrigger, 10)
		done := make(chan struct{})
		a.watchers[w.ID] = watcherHandle{cancel: cancel, trigger: trigger, updatedAt: updatedAt, done: done}
		generation := a.polling.register(w.ID)
		go func() {
			defer close(done)
			a.runWatcher(watcherCtx, w, trigger, generation)
		}()
	}
}

// runWatcher runs one watcher's immediate and ticker-triggered polling loop.
func (a *Agent) runWatcher(ctx context.Context, watcher *database.Watcher, trigger chan CheckTrigger, generation uint64) {
	defer a.polling.stop(watcher.ID, generation)
	log := a.log.WithWatcher(watcher.ID, watcher.Name)
	rw := NewRepoWatcher(watcher, a.db, a.appCfg, a.log, a.events, a.webhooks)
	log.Info("watcher starting",
		"service_name", watcher.ServiceName,
		"metadata_url", watcher.MetadataURL,
		"check_interval_sec", watcher.CheckIntervalSec,
		"services", len(watcher.Services),
	)

	run := func(request CheckTrigger) {
		trace, catalogID := request.Trace, request.CatalogID
		if ctx.Err() != nil {
			a.rejectCatalogTrigger(request, "catalog deployment interrupted before execution; review and retry")
			return
		}
		if watcher.Paused && catalogID == 0 {
			return
		}
		a.polling.checking(watcher.ID, generation, true)
		if a.events != nil {
			a.events.Publish(watcher.ID, WatcherEvent{Type: "poll_started", Data: map[string]any{"poll_id": trace.PollID}})
		}
		if catalogID != 0 {
			if err := rw.RunCatalog(WithTrace(ctx, trace), catalogID, request.DeployLogID); err != nil {
				log.WithTrace(trace).Error("catalog deployment failed", "error", err)
			}
		} else {
			_ = rw.Run(WithTrace(ctx, trace))
		}
		a.polling.checking(watcher.ID, generation, false)
		if a.events != nil {
			a.events.Publish(watcher.ID, WatcherEvent{Type: "poll_completed", Data: map[string]any{"poll_id": trace.PollID}})
		}
	}
	run(CheckTrigger{WatcherID: watcher.ID, Trace: NewPollTrace("", "startup")})

	timer := time.NewTimer(nextPollDelay(watcher.CheckIntervalSec))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("watcher stopping")
			return
		case <-timer.C:
			run(CheckTrigger{WatcherID: watcher.ID, Trace: NewPollTrace("", "scheduled")})
			timer.Reset(nextPollDelay(watcher.CheckIntervalSec))
		case request := <-trigger:
			log.WithTrace(request.Trace).Info("immediate check triggered via API")
			run(request)
		}
	}
}

// Reject a catalog request explicitly rather than leaving an open queued deployment.
func (a *Agent) rejectCatalogTrigger(trigger CheckTrigger, reason string) {
	if trigger.CatalogID == 0 {
		return
	}
	state := NewStateManager(a.db, trigger.WatcherID, a.log.WithTrace(trigger.Trace), a.events, a.webhooks)
	state.trace = trigger.Trace
	state.catalogID, state.deployLogID = trigger.CatalogID, trigger.DeployLogID
	if err := state.SetFailed(reason); err != nil {
		a.log.Error("failed to settle rejected catalog deployment", "watcher_id", trigger.WatcherID, "deploy_log_id", trigger.DeployLogID, "error", err)
	}
}

// A process restart loses queued work; keep the staged candidate and require a retry.
func (a *Agent) recoverCatalogDeployments() {
	var watchers []database.Watcher
	if err := a.db.Where("pending_catalog_id <> 0").Find(&watchers).Error; err != nil {
		a.log.Error("catalog recovery failed", "error", err)
		return
	}
	for _, watcher := range watchers {
		// Older processes can stop after closing the primary failure but before
		// completing its linked rollback. Settle that admission guard as well.
		var rollbacks []database.DeployLog
		if err := a.db.Where("watcher_id = ? AND kind = ? AND reason = ? AND completed_at IS NULL AND parent_attempt_id IN (SELECT id FROM deploy_logs WHERE watcher_id = ? AND kind = ? AND reason = ? AND version = ?)", watcher.ID, "rollback", "auto_after_deploy_failure", watcher.ID, "deploy", "catalog_release", watcher.PendingVersion).Find(&rollbacks).Error; err != nil {
			a.log.Error("catalog recovery failed to load rollback attempts", "watcher_id", watcher.ID, "error", err)
			continue
		}
		for _, rollback := range rollbacks {
			state := NewStateManager(a.db, watcher.ID, a.log, a.events, a.webhooks)
			if err := state.FailRollbackAttempt(rollback.ID, "catalog rollback interrupted by agent restart; inspect the service and retry"); err != nil {
				a.log.Error("failed to recover interrupted catalog rollback", "watcher_id", watcher.ID, "deploy_log_id", rollback.ID, "error", err)
			}
		}
		var attempts []database.DeployLog
		if err := a.db.Where("watcher_id = ? AND kind = ? AND reason = ? AND version = ? AND completed_at IS NULL", watcher.ID, "deploy", "catalog_release", watcher.PendingVersion).Find(&attempts).Error; err != nil {
			a.log.Error("catalog recovery failed to load queued attempts", "watcher_id", watcher.ID, "error", err)
			continue
		}
		for _, attempt := range attempts {
			state := NewStateManager(a.db, watcher.ID, a.log, a.events, a.webhooks)
			state.catalogID, state.deployLogID, state.recoverCatalog = watcher.PendingCatalogID, attempt.ID, true
			if err := state.SetFailed("catalog deployment interrupted by agent restart; inspect the service and retry"); err != nil {
				a.log.Error("failed to recover interrupted catalog deployment", "watcher_id", watcher.ID, "deploy_log_id", attempt.ID, "error", err)
			}
		}
		// Older versions closed the attempt and cleared approval in separate
		// writes. A restart between them leaves no open log for the loop above,
		// but must still make the retained candidate discardable.
		message := "catalog deployment interrupted by agent restart; inspect the service and retry"
		result := a.db.Model(&database.Watcher{}).
			Where("id = ? AND pending_catalog_id = ? AND pending_version = ? AND approved_version <> ''", watcher.ID, watcher.PendingCatalogID, watcher.PendingVersion).
			Where("NOT EXISTS (SELECT 1 FROM deploy_logs WHERE watcher_id = ? AND completed_at IS NULL)", watcher.ID).
			UpdateColumns(map[string]any{"approved_version": "", "status": string(StatusFailed), "last_error": message})
		if result.Error != nil {
			a.log.Error("failed to release interrupted catalog approval", "watcher_id", watcher.ID, "error", result.Error)
		} else if result.RowsAffected != 0 && a.events != nil {
			a.events.Publish(watcher.ID, WatcherEvent{Type: EventStatusChanged, Data: map[string]any{"status": string(StatusFailed), "error": message}})
		}
	}
}

// stopWatcher joins compensation as well as the canceled deployment before replacement.
func (a *Agent) stopWatcher(h watcherHandle, reason string) {
	h.cancel()
	<-h.done
	a.drainCatalogTriggers(h.trigger, reason)
}

func (a *Agent) stopWatchers() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, h := range a.watchers {
		h.cancel()
	}
	for id, h := range a.watchers {
		a.stopWatcher(h, "catalog deployment interrupted by agent shutdown; review and retry")
		delete(a.watchers, id)
	}
}

func (a *Agent) drainCatalogTriggers(queue <-chan CheckTrigger, reason string) {
	for {
		select {
		case request, ok := <-queue:
			if !ok {
				return
			}
			a.rejectCatalogTrigger(request, reason)
		default:
			return
		}
	}
}

func (a *Agent) shutdown() {
	a.stopWatchers()
	a.drainCatalogTriggers(a.checkTrigger, "catalog deployment interrupted by agent shutdown; review and retry")
}
