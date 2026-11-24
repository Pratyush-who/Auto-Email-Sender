package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Pinger interface for checking database connection
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler manages the /health endpoint
type HealthHandler struct {
	db        Pinger
	startTime time.Time
}

// NewHealthHandler creates a HealthHandler
func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{
		db:        db,
		startTime: time.Now(),
	}
}

// HealthResponse represents response payload for health check
type HealthResponse struct {
	Status    string `json:"status"`
	Database  string `json:"database"`
	Uptime    string `json:"uptime"`
	Timestamp string `json:"timestamp"`
	Error     string `json:"error,omitempty"`
}

// ServeHTTP handles GET /health
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	dbStatus := "connected"
	var errStr string
	statusCode := http.StatusOK

	if h.db != nil {
		if err := h.db.Ping(ctx); err != nil {
			dbStatus = "disconnected"
			errStr = err.Error()
			statusCode = http.StatusServiceUnavailable
		}
	} else {
		dbStatus = "unconfigured"
	}

	overallStatus := "ok"
	if dbStatus == "disconnected" {
		overallStatus = "degraded"
	}

	resp := HealthResponse{
		Status:    overallStatus,
		Database:  dbStatus,
		Uptime:    time.Since(h.startTime).Truncate(time.Second).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Error:     errStr,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(resp)
}
