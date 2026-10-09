package api

import (
	"context"
	"os/exec"
	"runtime"
	"time"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/webhook"
	"gorm.io/gorm"
)

type Handler struct {
	polling        *agent.PollingMonitor
	db             *gorm.DB
	nssmPath       string
	serviceManager agent.ServiceManager
	logDir         string
	version        string
	githubToken    string
	envPath        string
	appCfg         *config.AppConfig
	log            *agent.Logger
	events         *agent.WatcherEventBus
	startTime      time.Time
	checkTrigger   chan agent.CheckTrigger // send watcher ID for immediate poll
	syncTrigger    chan struct{}           // trigger background agent to sync DB
	webhooks       *webhook.Service
	webhookTrigger chan struct{}
	isWindows      func() bool
	runNSSMCommand func(context.Context, ...string) ([]byte, error)
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(db *gorm.DB, nssmPath, logDir, version, githubToken, envPath string, appCfg *config.AppConfig, log *agent.Logger, events *agent.WatcherEventBus, checkTrigger chan agent.CheckTrigger, syncTrigger chan struct{}, webhookService *webhook.Service, webhookTrigger chan struct{}) *Handler {
	if log == nil {
		log = agent.NewLogger("api")
	}
	return &Handler{
		db:             db,
		nssmPath:       nssmPath,
		serviceManager: agent.NewNSSMServiceManager(nssmPath),
		logDir:         logDir,
		version:        version,
		githubToken:    githubToken,
		envPath:        envPath,
		appCfg:         appCfg,
		log:            log,
		events:         events,
		startTime:      time.Now(),
		checkTrigger:   checkTrigger,
		syncTrigger:    syncTrigger,
		webhooks:       webhookService,
		webhookTrigger: webhookTrigger,
		isWindows:      func() bool { return runtime.GOOS == "windows" },
		runNSSMCommand: func(ctx context.Context, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, nssmPath, args...).CombinedOutput()
		},
	}
}

// ── Watcher CRUD ──────────────────────────────────────────────

// ListWatchers returns all watchers with their services and current state.
