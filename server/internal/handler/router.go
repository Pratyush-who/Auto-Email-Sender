package handler

import (
	"log/slog"
	"net/http"

	"emailauto-server/internal/config"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// SetupRouter initializes Chi router, middlewares, CORS, and registers routes.
func SetupRouter(cfg *config.Config, logger *slog.Logger, db Pinger) *chi.Mux {
	r := chi.NewRouter()

	// Base middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(StructuredLoggerMiddleware(logger))

	// CORS configuration for Vite dev origin
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSAllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Requested-With"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check endpoint
	healthHandler := NewHealthHandler(db)
	r.Get("/health", healthHandler.ServeHTTP)

	// Base API route placeholder
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"service":"emailauto-server","version":"1.0.0"}`))
	})

	return r
}
