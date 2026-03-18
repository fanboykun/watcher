package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/fanboykun/watcher/internal"
)

func main() {
	configPath := flag.String("config", "config.json", "path to config.json")
	flag.Parse()

	log := internal.NewLogger("agent")
	log.Info("watcher agent starting", "config", *configPath)

	cfg, err := internal.LoadConfig(*configPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Switch to file logger now that we have log dir
	log, err = internal.NewFileLogger("agent", cfg.LogDir)
	if err != nil {
		log = internal.NewLogger("agent")
		log.Warn("could not open log file, using stdout only", "error", err)
	}

	log.Info("config loaded",
		"environment", cfg.Environment,
		"watchers", len(cfg.Watchers),
	)
	for _, w := range cfg.Watchers {
		log.Info("watcher registered",
			"name", w.Name,
			"service_name", w.ServiceName,
			"check_interval_sec", w.CheckIntervalSec,
			"services", len(w.Services),
		)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	agent := internal.NewAgent(cfg, log)
	agent.Run(ctx) // blocks until ctx cancelled — one goroutine per watcher
}