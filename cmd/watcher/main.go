package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fanboykun/watcher/internal"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config.json")
	flag.Parse()

	// Bootstrap logger to stdout until we know the log dir
	log := internal.NewLogger("watcher")
	log.Info("watcher starting", "config", *configPath)

	cfg, err := internal.LoadConfig(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Switch to file logger now that we have log dir
	log, err = internal.NewFileLogger("watcher", cfg.LogDir)
	if err != nil {
		// Non-fatal — fall back to stdout-only
		log = internal.NewLogger("watcher")
		log.Warn("could not open log file, using stdout only", "error", err)
	}

	log.Info("config loaded",
		"service_name", cfg.ServiceName,
		"environment", cfg.Environment,
		"install_dir", cfg.InstallDir,
		"check_interval_sec", cfg.CheckIntervalSec,
		"managed_services", len(cfg.Services),
	)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	watcher := internal.NewWatcher(cfg, log)

	// Run first check immediately on startup
	log.Info("running initial check")
	if err := watcher.Run(ctx); err != nil && err != context.Canceled {
		log.Error("initial check failed", "error", err)
	}

	ticker := time.NewTicker(time.Duration(cfg.CheckIntervalSec) * time.Second)
	defer ticker.Stop()

	log.Info("entering poll loop", "interval_sec", cfg.CheckIntervalSec)

	for {
		select {
		case <-ctx.Done():
			log.Info("shutdown signal received, exiting")
			return
		case <-ticker.C:
			if err := watcher.Run(ctx); err != nil && err != context.Canceled {
				log.Error("check cycle failed", "error", err)
				// Non-fatal — keep polling, will retry next tick
			}
		}
	}
}