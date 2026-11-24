package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"emailauto-server/internal/config"
	"emailauto-server/internal/database"
	"emailauto-server/internal/handler"
	"emailauto-server/internal/logging"
)

func main() {
	cfg := config.Load()
	logger := logging.SetupLogger(cfg.LogLevel, cfg.Environment)

	logger.Info("starting emailauto server",
		"port", cfg.Port,
		"env", cfg.Environment,
		"cors_origins", cfg.CORSAllowedOrigins,
	)

	// Attempt database connection with retries (useful for docker-compose startup)
	ctx := context.Background()
	var db *database.DB
	var dbErr error

	maxRetries := 5
	for i := 1; i <= maxRetries; i++ {
		db, dbErr = database.Connect(ctx, cfg.DatabaseURL)
		if dbErr == nil {
			logger.Info("connected to postgres database successfully")
			break
		}
		logger.Warn("waiting for postgres to be ready...",
			"attempt", i,
			"max_retries", maxRetries,
			"error", dbErr,
		)
		time.Sleep(2 * time.Second)
	}

	if dbErr != nil {
		logger.Error("could not connect to postgres database", "error", dbErr)
		// We still allow server to start so /health returns degraded instead of fatal crash,
		// or fatal if required in production.
	} else {
		defer db.Close()

		if cfg.MigrateOnStartup {
			if err := database.RunMigrations(cfg.DatabaseURL, logger); err != nil {
				logger.Error("migration execution failed", "error", err)
				os.Exit(1)
			}
		}
	}

	r := handler.SetupRouter(cfg, logger, db)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Server run context
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info(fmt.Sprintf("http server listening on :%s", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("server error encountered", "error", err)
		os.Exit(1)
	case sig := <-shutdown:
		logger.Info("shutdown signal received", "signal", sig.String())

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed, forcing close", "error", err)
			_ = srv.Close()
		}
		logger.Info("server exited cleanly")
	}
}
