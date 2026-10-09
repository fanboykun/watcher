package agent

import (
	"context"
	"sync"

	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
	"github.com/fanboykun/watcher/internal/webhook"
	"gorm.io/gorm"
)

type watcherHandle struct {
	cancel    context.CancelFunc
	trigger   chan CheckTrigger
	updatedAt int64
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
	}
}

// Run starts the agent supervisor and blocks until its context is cancelled.
func (a *Agent) Run(ctx context.Context) {
	a.prepareConfigSnapshots()
	a.syncWatchers(ctx)

	for {
		select {
		case <-ctx.Done():
			a.mu.Lock()
			for _, h := range a.watchers {
				h.cancel()
			}
			a.mu.Unlock()
			a.log.Info("all watchers stopped")
			return
		case <-a.syncTrigger:
			a.log.Info("syncing watchers from database")
			a.syncWatchers(ctx)
		case trigger := <-a.checkTrigger:
			a.mu.Lock()
			h, ok := a.watchers[trigger.WatcherID]
			a.mu.Unlock()
			if ok {
				select {
				case h.trigger <- trigger:
				default:
					a.log.WithTrace(trigger.Trace).Warn("poll trigger skipped: watcher queue is full", "watcher_id", trigger.WatcherID)
				}
			} else {
				a.log.WithTrace(trigger.Trace).Warn("check trigger for unknown watcher", "watcher_id", trigger.WatcherID)
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
			h.cancel()
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
			h.cancel()
		} else {
			a.log.Info("starting new watcher", "id", w.ID)
		}
		watcherCtx, cancel := context.WithCancel(ctx)
		trigger := make(chan CheckTrigger, 10)
		a.watchers[w.ID] = watcherHandle{cancel: cancel, trigger: trigger, updatedAt: updatedAt}
		generation := a.polling.register(w.ID)
		go a.runWatcher(watcherCtx, w, trigger, generation)
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

	run := func(trace Trace) {
		if watcher.Paused {
			return
		}
		a.polling.checking(watcher.ID, generation, true)
		if a.events != nil {
			a.events.Publish(watcher.ID, WatcherEvent{Type: "poll_started", Data: map[string]any{"poll_id": trace.PollID}})
		}
		_ = rw.Run(WithTrace(ctx, trace))
		a.polling.checking(watcher.ID, generation, false)
		if a.events != nil {
			a.events.Publish(watcher.ID, WatcherEvent{Type: "poll_completed", Data: map[string]any{"poll_id": trace.PollID}})
		}
	}
	run(NewPollTrace("", "startup"))

	ticker := newTicker(watcher.CheckIntervalSec)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("watcher stopping")
			return
		case <-ticker.C:
			run(NewPollTrace("", "scheduled"))
		case request := <-trigger:
			log.WithTrace(request.Trace).Info("immediate check triggered via API")
			run(request.Trace)
		}
	}
}
