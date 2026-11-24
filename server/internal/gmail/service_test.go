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

type mockTokenStore struct {
	data *OAuthTokenData
}

func (m *mockTokenStore) SaveToken(ctx context.Context, data *OAuthTokenData) error {
	m.data = data
	return nil
}

func (m *mockTokenStore) GetToken(ctx context.Context) (*OAuthTokenData, error) {
	return m.data, nil
}

func (m *mockTokenStore) DeleteToken(ctx context.Context) error {
	m.data = nil
	return nil
}

func TestGmailService_GetAuthURL(t *testing.T) {
	svc := NewGmailService(ServiceConfig{
		ClientID:    "client-123.apps.googleusercontent.com",
		RedirectURL: "http://localhost:8080/auth/callback",
	}, nil)

	authURL := svc.GetAuthURL("sample-state")

	if !strings.Contains(authURL, "client_id=client-123.apps.googleusercontent.com") {
		t.Errorf("missing client_id in auth url")
	}
	if !strings.Contains(authURL, "access_type=offline") {
		t.Errorf("missing access_type=offline")
	}
	if !strings.Contains(authURL, "prompt=consent") {
		t.Errorf("missing prompt=consent")
	}
	if !strings.Contains(authURL, "gmail.send") || !strings.Contains(authURL, "gmail.readonly") || !strings.Contains(authURL, "userinfo.email") {
		t.Errorf("missing required scopes in auth url")
	}
}

func TestGmailService_HandleCallback(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"access_token": "ya29.test-access-token",
			"refresh_token": "1//test-refresh-token",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer tokenServer.Close()

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"email": "candidate@example.com"}`))
	}))
	defer userinfoServer.Close()

	store := &mockTokenStore{}
	key := "test-aes-key-for-oauth"

	svc := NewGmailService(ServiceConfig{
		ClientID:      "test-client",
		ClientSecret:  "test-secret",
		RedirectURL:   "http://localhost:8080/auth/callback",
		EncryptionKey: key,
		TokenURL:      tokenServer.URL,
		UserInfoURL:   userinfoServer.URL,
	}, store)

	tokenData, err := svc.HandleCallback(context.Background(), "auth-code-123")
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}

	if tokenData.AccountEmail != "candidate@example.com" {
		t.Errorf("unexpected account email: %s", tokenData.AccountEmail)
	}

	// Verify refresh token was encrypted with AES-GCM
	decryptedRefresh, err := DecryptAESGCM(tokenData.EncryptedRefreshToken, key)
	if err != nil {
		t.Fatalf("failed to decrypt stored refresh token: %v", err)
	}
	if decryptedRefresh != "1//test-refresh-token" {
		t.Errorf("unexpected decrypted refresh token: %s", decryptedRefresh)
	}

	// Verify status
	status, err := svc.GetStatus(context.Background())
	if err != nil || !status.Connected || status.Email != "candidate@example.com" {
		t.Errorf("status not connected as expected: %+v", status)
	}
}

func TestGmailService_SendMail(t *testing.T) {
	mockGmailAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/messages/send") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id": "msg_abc123", "threadId": "th_xyz789"}`))
	}))
	defer mockGmailAPI.Close()

	store := &mockTokenStore{
		data: &OAuthTokenData{
			AccountEmail: "candidate@example.com",
			AccessToken:  "valid-token",
			Expiry:       time.Now().UTC().Add(1 * time.Hour),
		},
	}

	svc := NewGmailService(ServiceConfig{
		GmailBaseURL: mockGmailAPI.URL,
	}, store)

	msgID, threadID, err := svc.SendMail(
		context.Background(),
		"hiring@company.com",
		"Job Application",
		"Hello, please find my resume attached.",
		nil,
		"",
		"",
		"th_xyz789",
	)
	if err != nil {
		t.Fatalf("SendMail failed: %v", err)
	}

	if msgID != "msg_abc123" || threadID != "th_xyz789" {
		t.Errorf("unexpected SendMail return: id=%s, threadId=%s", msgID, threadID)
	}
}

func TestGmailService_CheckThreads_InboundAndBounce(t *testing.T) {
	mockGmailAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		thread := gmailThread{
			ID: "th_001",
			Messages: []gmailMessage{
				{
					ID:           "msg_outbound",
					ThreadID:     "th_001",
					Snippet:      "Applying for engineer role",
					InternalDate: "1732450000000",
					Payload: gmailMessagePayload{
						Headers: []gmailHeader{
							{Name: "From", Value: "Candidate <candidate@example.com>"},
							{Name: "To", Value: "hr@company.com"},
						},
					},
				},
				{
					ID:           "msg_reply",
					ThreadID:     "th_001",
					Snippet:      "Thanks for applying, let's schedule an interview",
					InternalDate: "1732453600000",
					Payload: gmailMessagePayload{
						Headers: []gmailHeader{
							{Name: "From", Value: "Recruiter <hr@company.com>"},
							{Name: "To", Value: "candidate@example.com"},
							{Name: "Subject", Value: "Re: Applying for engineer role"},
						},
					},
				},
				{
					ID:           "msg_bounce",
					ThreadID:     "th_001",
					Snippet:      "Delivery Status Notification (Failure) - Mailbox full",
					InternalDate: "1732454000000",
					Payload: gmailMessagePayload{
						Headers: []gmailHeader{
							{Name: "From", Value: "Mail Delivery Subsystem <mailer-daemon@googlemail.com>"},
							{Name: "To", Value: "candidate@example.com"},
							{Name: "Subject", Value: "Delivery Status Notification (Failure)"},
						},
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(thread)
	}))
	defer mockGmailAPI.Close()

	store := &mockTokenStore{
		data: &OAuthTokenData{
			AccountEmail: "candidate@example.com",
			AccessToken:  "valid-token",
			Expiry:       time.Now().UTC().Add(1 * time.Hour),
		},
	}

	svc := NewGmailService(ServiceConfig{
		GmailBaseURL: mockGmailAPI.URL,
	}, store)

	inbounds, err := svc.CheckThreads(context.Background(), []string{"th_001"})
	if err != nil {
		t.Fatalf("CheckThreads failed: %v", err)
	}

	// Should ignore outbound message from me, returning 2 inbound messages
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbound messages, got %d", len(inbounds))
	}

	// First should be normal reply
	if inbounds[0].IsBounce {
		t.Errorf("first inbound should not be bounce")
	}
	if inbounds[0].From != "Recruiter <hr@company.com>" {
		t.Errorf("unexpected sender: %s", inbounds[0].From)
	}

	// Second should be detected bounce
	if !inbounds[1].IsBounce {
		t.Errorf("second inbound should be detected as bounce")
	}
	if inbounds[1].BounceReason == "" {
		t.Errorf("expected bounceReason to be set")
	}
}

func TestDetectBounce(t *testing.T) {
	tests := []struct {
		name         string
		from         string
		subject      string
		snippet      string
		headers      map[string]string
		expectBounce bool
	}{
		{
			name:         "Mailer-daemon sender",
			from:         "mailer-daemon@google.com",
			subject:      "Failure notice",
			snippet:      "Unable to deliver",
			headers:      map[string]string{},
			expectBounce: true,
		},
		{
			name:         "Delivery Status Notification Subject",
			from:         "system@domain.com",
			subject:      "Delivery Status Notification (Failure)",
			snippet:      "550 User unknown",
			headers:      map[string]string{},
			expectBounce: true,
		},
		{
			name:         "Action failed header",
			from:         "service@domain.com",
			subject:      "Status report",
			snippet:      "Info",
			headers:      map[string]string{"action": "failed"},
			expectBounce: true,
		},
		{
			name:         "Legitimate inbound email",
			from:         "founder@startup.io",
			subject:      "Re: Collaboration proposal",
			snippet:      "Sounds interesting, let's talk next week.",
			headers:      map[string]string{},
			expectBounce: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isBounce, reason := DetectBounce(tt.from, tt.subject, tt.snippet, tt.headers)
			if isBounce != tt.expectBounce {
				t.Errorf("DetectBounce() = %v, want %v (reason: %q)", isBounce, tt.expectBounce, reason)
			}
		})
	}
}
