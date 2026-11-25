package handler

import (
	"log/slog"
	"net/http"

	"emailauto-server/internal/config"
	"emailauto-server/internal/gmail"
	"emailauto-server/internal/sender"
	"emailauto-server/internal/worker"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// RouterDependencies holds stores and services needed to construct the HTTP router.
type RouterDependencies struct {
	Config        *config.Config
	Logger        *slog.Logger
	DB            Pinger
	ProfileStore  ProfileStore
	SettingsStore SettingsStore
	PostStore     PostStore
	ContactStore  ContactStore
	OutreachStore OutreachStore
	ReplyStore    ReplyStore
	Composer      ComposerService
	Sender        *sender.Sender
	Pipeline      PostPipeline
	Orchestrator  OrchestratorController
	Poller        PollerController
	GmailService  *gmail.GmailService
	WorkerClient  *worker.Client
}

// SetupRouter initializes Chi router, middlewares, CORS, and registers all routes.
func SetupRouter(deps RouterDependencies) *chi.Mux {
	r := chi.NewRouter()

	// Base middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(StructuredLoggerMiddleware(deps.Logger))

	// CORS configuration for Vite dev origin
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   deps.Config.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Requested-With"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check endpoint
	healthHandler := NewHealthHandler(deps.DB)
	r.Get("/health", healthHandler.ServeHTTP)

	// Authentication Endpoints (/auth/google, /auth/callback, /auth/status, /auth/disconnect)
	if deps.GmailService != nil {
		authHandler := gmail.NewAuthHandler(deps.GmailService, deps.Config.FrontendURL)
		r.Route("/auth", func(auth chi.Router) {
			auth.Get("/google", authHandler.HandleGoogleAuth)
			auth.Get("/callback", authHandler.HandleCallback)
			auth.Get("/status", authHandler.HandleStatus)
			auth.Post("/disconnect", authHandler.HandleDisconnect)
		})
	}

	// API Handlers
	settingsHandler := NewSettingsHandler(deps.SettingsStore)
	profileHandler := NewProfileHandler(deps.ProfileStore, deps.Config.UploadDir)
	postHandler := NewPostHandler(deps.PostStore, deps.ContactStore, deps.SettingsStore, deps.WorkerClient, deps.Pipeline)
	outreachHandler := NewOutreachHandler(deps.OutreachStore, deps.ProfileStore, deps.PostStore, deps.ContactStore, deps.Composer)
	senderHandler := NewSenderHandler(deps.Sender)
	orchestratorHandler := NewOrchestratorHandler(deps.Orchestrator, deps.ReplyStore, deps.Poller)

	// REST API Routes
	r.Route("/api", func(api chi.Router) {
		// Settings
		api.Get("/settings", settingsHandler.GetSettings)
		api.Put("/settings", settingsHandler.UpdateSettings)

		// Profile
		api.Get("/profile", profileHandler.GetProfile)
		api.Put("/profile", profileHandler.UpdateProfile)
		api.Post("/profile/resume", profileHandler.UploadResume)

		// Posts & Scrape Runs
		api.Post("/posts/manual", postHandler.ManualImportPost)
		api.Get("/posts", postHandler.ListPosts)
		api.Delete("/posts", postHandler.ClearPosts)
		api.Post("/runs", postHandler.TriggerRun)
		api.Get("/runs", postHandler.ListRuns)

		// Contacts
		api.Get("/contacts", postHandler.ListContacts)

		// Outreach & Composer
		api.Get("/outreach", outreachHandler.ListOutreach)
		api.Get("/outreach/{id}", outreachHandler.GetOutreach)
		api.Put("/outreach/{id}", outreachHandler.UpdateOutreach)
		api.Post("/outreach/{id}/approve", outreachHandler.ApproveOutreach)
		api.Post("/outreach/{id}/skip", outreachHandler.SkipOutreach)
		api.Post("/composer/generate", outreachHandler.GenerateOutreach)

		// Sender Controls
		api.Get("/sender/status", senderHandler.GetStatus)
		api.Post("/sender/pause", senderHandler.Pause)
		api.Post("/sender/resume", senderHandler.Resume)
		api.Post("/sender/stop-all", senderHandler.StopAll)
		api.Post("/sender/send-next", senderHandler.SendNext)

		// Orchestrator, Auto-Schedule & Replies
		api.Get("/orchestrator/status", orchestratorHandler.GetStatus)
		api.Post("/orchestrator/toggle-auto", orchestratorHandler.ToggleAuto)
		api.Post("/orchestrator/kill-switch", orchestratorHandler.KillSwitch)
		api.Post("/orchestrator/run-now", orchestratorHandler.RunNow)
		api.Get("/replies", orchestratorHandler.ListReplies)
		api.Post("/poller/poll", orchestratorHandler.PollNow)
	})

	// Base root ping
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"emailauto-server","version":"1.0.0"}`))
	})

	return r
}
