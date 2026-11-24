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

	"emailauto-server/internal/composer"
	"emailauto-server/internal/config"
	"emailauto-server/internal/database"
	"emailauto-server/internal/gmail"
	"emailauto-server/internal/handler"
	"emailauto-server/internal/llm"
	"emailauto-server/internal/logging"
	"emailauto-server/internal/orchestrator"
	"emailauto-server/internal/pipeline"
	"emailauto-server/internal/poller"
	"emailauto-server/internal/repository"
	"emailauto-server/internal/sender"
	"emailauto-server/internal/worker"
)

func main() {
	cfg := config.Load()
	logger := logging.SetupLogger(cfg.LogLevel, cfg.Environment)

	logger.Info("starting emailauto server",
		"port", cfg.Port,
		"env", cfg.Environment,
		"cors_origins", cfg.CORSAllowedOrigins,
		"upload_dir", cfg.UploadDir,
		"worker_url", cfg.WorkerURL,
	)

	// Ensure upload directory exists
	if err := os.MkdirAll(cfg.UploadDir, 0755); err != nil {
		logger.Warn("could not create upload directory", "dir", cfg.UploadDir, "error", err)
	}

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

	var profileStore handler.ProfileStore
	var settingsStore handler.SettingsStore
	var postStore handler.PostStore
	var postRepo *repository.PostRepository
	var contactRepo *repository.ContactRepository
	var outreachRepo *repository.OutreachRepository
	var replyRepo *repository.ReplyRepository
	var tokenStore gmail.TokenStore
	var settingsRepo *repository.SettingsRepository

	if dbErr != nil {
		logger.Error("could not connect to postgres database", "error", dbErr)
	} else {
		defer db.Close()

		if cfg.MigrateOnStartup {
			if err := database.RunMigrations(cfg.DatabaseURL, logger); err != nil {
				logger.Error("migration execution failed", "error", err)
				os.Exit(1)
			}
		}

		profileStore = repository.NewProfileRepository(db.Pool)
		settingsRepo = repository.NewSettingsRepository(db.Pool)
		settingsStore = settingsRepo
		postRepo = repository.NewPostRepository(db.Pool)
		postStore = postRepo
		contactRepo = repository.NewContactRepository(db.Pool)
		outreachRepo = repository.NewOutreachRepository(db.Pool)
		replyRepo = repository.NewReplyRepository(db.Pool)
		tokenStore = repository.NewTokenRepository(db.Pool)
	}

	// Initialize Gemini LLM client if configured
	var geminiClient llm.Client
	if cfg.GeminiAPIKey != "" {
		geminiClient = llm.NewGeminiClient(llm.GeminiConfig{
			APIKey:     cfg.GeminiAPIKey,
			Model:      cfg.GeminiModel,
			RPM:        cfg.GeminiRPM,
			Timeout:    cfg.GeminiTimeout,
			Logger:     logger,
		})
	}

	// Initialize Composer service
	var composerService *composer.Composer
	if geminiClient != nil {
		composerService = composer.NewComposer(geminiClient, logger)
	}

	// Initialize Extraction & Verification Pipeline
	var pipelineService *pipeline.Pipeline
	if geminiClient != nil {
		domainCache := pipeline.NewDomainCache(settingsRepo)
		extractor := pipeline.NewPostExtractor(geminiClient)
		verifier := pipeline.NewCompanyVerifier(geminiClient, domainCache, nil, nil, logger)
		pipelineService = pipeline.NewPipeline(extractor, verifier, postStore, contactRepo, logger)
		if composerService != nil {
			pipelineService.WithComposer(composerService, profileStore, outreachRepo)
		}
	}

	// Initialize Gmail service
	gmailService := gmail.NewGmailService(gmail.ServiceConfig{
		ClientID:      cfg.GoogleClientID,
		ClientSecret:  cfg.GoogleClientSecret,
		RedirectURL:   cfg.GoogleRedirectURL,
		EncryptionKey: cfg.EncryptionKey,
		Logger:        logger,
	}, tokenStore)

	// Initialize Sender service
	senderService := sender.NewSender(outreachRepo, profileStore, settingsStore, gmailService, logger)

	// Initialize Worker client
	workerClient := worker.NewClient(cfg.WorkerURL)

	// Initialize 30-minute Gmail thread poller
	pollerService := poller.NewPoller(
		30*time.Minute,
		outreachRepo,
		replyRepo,
		settingsRepo,
		gmailService,
		logger,
	)
	pollerService.Start()
	defer pollerService.Stop()

	// Initialize 2-hour Orchestrator state machine
	orchestratorService := orchestrator.NewOrchestrator(
		settingsRepo,
		postRepo,
		workerClient,
		pipelineService,
		outreachRepo,
		logger,
	)
	_ = orchestratorService.InitFromDB(ctx)
	orchestratorService.Start()
	defer orchestratorService.Stop()

	r := handler.SetupRouter(handler.RouterDependencies{
		Config:        cfg,
		Logger:        logger,
		DB:            db,
		ProfileStore:  profileStore,
		SettingsStore: settingsStore,
		PostStore:     postStore,
		ContactStore:  contactRepo,
		OutreachStore: outreachRepo,
		ReplyStore:    replyRepo,
		Composer:      composerService,
		Sender:        senderService,
		Pipeline:      pipelineService,
		Orchestrator:  orchestratorService,
		Poller:        pollerService,
		GmailService:  gmailService,
		WorkerClient:  workerClient,
	})

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
