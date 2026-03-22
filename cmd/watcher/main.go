package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fanboykun/watcher/internal/agent"
	"github.com/fanboykun/watcher/internal/api"
	"github.com/fanboykun/watcher/internal/config"
	"github.com/fanboykun/watcher/internal/database"
)

var Version = "dev"

func main() {
	envPath := flag.String("config", ".env", "path to .env config file")
	flag.Parse()

	log := agent.NewLogger("agent")
	log.Info("watcher agent starting", "version", Version, "config", *envPath)

	cfg, err := config.LoadConfig(*envPath)
	if err != nil {
		log.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Switch to file logger now that we have log dir
	log, err = agent.NewFileLogger("agent", cfg.LogDir)
	if err != nil {
		log = agent.NewLogger("agent")
		log.Warn("could not open log file, using stdout only", "error", err)
	}

	log.Info("config loaded",
		"environment", cfg.Environment,
		"db_path", cfg.DBPath,
		"api_port", cfg.APIPort,
	)

	// Initialize database
	db, err := database.NewDB(cfg.DBPath)
	if err != nil {
		log.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	log.Info("database ready", "path", cfg.DBPath)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Channel for API → Agent immediate check triggers
	checkTrigger := make(chan uint, 10)

	// Channel for API → Agent to trigger a config reload
	syncTrigger := make(chan struct{}, 1)

	// Start API server in background
	router := api.NewRouter(db, cfg.NssmPath, cfg.LogDir, Version, cfg.GitHubToken, checkTrigger, syncTrigger)
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.APIPort),
		Handler: router,
	}

	go func() {
		log.Info("API server starting", "port", cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("API server error", "error", err)
		}
	}()

	// Start watcher agent (blocks until ctx cancelled)
	a := agent.NewAgent(db, cfg, log, checkTrigger, syncTrigger)
	a.Run(ctx)

	// Graceful shutdown of API server
	log.Info("shutting down API server")
	if err := srv.Shutdown(context.Background()); err != nil {
		log.Warn("API server shutdown error", "error", err)
	}
}