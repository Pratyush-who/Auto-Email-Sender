package gmail

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// GmailManager is the interface consumed by the HTTP auth handlers.
type GmailManager interface {
	GetAuthURL(state string) string
	HandleCallback(r *http.Request, code string) (*OAuthTokenData, error)
	GetStatus(r *http.Request) (*AuthStatus, error)
	Disconnect(r *http.Request) error
}

// AuthHandler provides endpoints for /auth/google, /auth/callback, /auth/status, /auth/disconnect.
type AuthHandler struct {
	service     *GmailService
	frontendURL string
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(service *GmailService, frontendURL string) *AuthHandler {
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	return &AuthHandler{
		service:     service,
		frontendURL: frontendURL,
	}
}

// HandleGoogleAuth initiates OAuth 2.0 flow (GET /auth/google).
func (h *AuthHandler) HandleGoogleAuth(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 16)
	_, _ = rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	authURL := h.service.GetAuthURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// HandleCallback processes the Google OAuth callback (GET /auth/callback).
func (h *AuthHandler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	errParam := r.URL.Query().Get("error")
	if errParam != "" {
		redirectWithError(w, r, h.frontendURL, "Google sign-in was denied: "+errParam)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		redirectWithError(w, r, h.frontendURL, "Missing authorization code from Google")
		return
	}

	tokenData, err := h.service.HandleCallback(r.Context(), code)
	if err != nil {
		redirectWithError(w, r, h.frontendURL, "OAuth token exchange failed: "+err.Error())
		return
	}

	// Successful redirect back to frontend
	destURL := fmt.Sprintf("%s/?auth=success&email=%s", h.frontendURL, url.QueryEscape(tokenData.AccountEmail))
	http.Redirect(w, r, destURL, http.StatusTemporaryRedirect)
}

func redirectWithError(w http.ResponseWriter, r *http.Request, frontendURL, errMsg string) {
	destURL := fmt.Sprintf("%s/?auth=error&error=%s", frontendURL, url.QueryEscape(errMsg))
	http.Redirect(w, r, destURL, http.StatusTemporaryRedirect)
}

// HandleStatus returns the Google connection status (GET /auth/status).
func (h *AuthHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.GetStatus(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

// HandleDisconnect disconnects and revokes tokens (POST /auth/disconnect).
func (h *AuthHandler) HandleDisconnect(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Disconnect(r.Context()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "disconnected"})
}
