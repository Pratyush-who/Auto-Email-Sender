package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthHandler_HandleGoogleAuth(t *testing.T) {
	svc := NewGmailService(ServiceConfig{
		ClientID:    "client-id-test",
		RedirectURL: "http://localhost:8080/auth/callback",
	}, nil)

	handler := NewAuthHandler(svc, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
	rec := httptest.NewRecorder()

	handler.HandleGoogleAuth(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect status 307, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "accounts.google.com") || !strings.Contains(location, "client_id=client-id-test") {
		t.Errorf("unexpected redirect location: %s", location)
	}
}

func TestAuthHandler_HandleCallback_Error(t *testing.T) {
	svc := NewGmailService(ServiceConfig{}, nil)
	handler := NewAuthHandler(svc, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?error=access_denied", nil)
	rec := httptest.NewRecorder()

	handler.HandleCallback(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected redirect status 307, got %d", rec.Code)
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "auth=error") || !strings.Contains(location, "access_denied") {
		t.Errorf("expected error redirect to frontend, got: %s", location)
	}
}

func TestAuthHandler_HandleStatus(t *testing.T) {
	now := time.Now().UTC()
	store := &mockTokenStore{
		data: &OAuthTokenData{
			AccountEmail: "user@example.com",
			Scopes:       GoogleOAuthScopes,
			UpdatedAt:    now,
		},
	}

	svc := NewGmailService(ServiceConfig{}, store)
	handler := NewAuthHandler(svc, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	rec := httptest.NewRecorder()

	handler.HandleStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var status AuthStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode response json: %v", err)
	}

	if !status.Connected || status.Email != "user@example.com" {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestAuthHandler_HandleDisconnect(t *testing.T) {
	store := &mockTokenStore{
		data: &OAuthTokenData{
			AccountEmail: "user@example.com",
		},
	}

	svc := NewGmailService(ServiceConfig{}, store)
	handler := NewAuthHandler(svc, "http://localhost:5173")

	req := httptest.NewRequest(http.MethodPost, "/auth/disconnect", nil)
	rec := httptest.NewRecorder()

	handler.HandleDisconnect(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["status"] != "disconnected" {
		t.Errorf("unexpected disconnect response: %+v", resp)
	}

	// Verify token removed from store
	token, _ := store.GetToken(context.Background())
	if token != nil {
		t.Errorf("token should have been deleted from store")
	}
}
