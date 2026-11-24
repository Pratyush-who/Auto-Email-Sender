package gmail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ServiceConfig holds credentials and endpoints for GmailService.
type ServiceConfig struct {
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	EncryptionKey string
	AuthURL       string // default: "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL      string // default: "https://oauth2.googleapis.com/token"
	UserInfoURL   string // default: "https://www.googleapis.com/oauth2/v3/userinfo"
	RevokeURL     string // default: "https://oauth2.googleapis.com/revoke"
	GmailBaseURL  string // default: "https://gmail.googleapis.com/gmail/v1/users/me"
	HTTPClient    *http.Client
	Logger        *slog.Logger
}

// GmailService handles Google OAuth2, token lifecycle, email sending, and thread inspection.
type GmailService struct {
	clientID      string
	clientSecret  string
	redirectURL   string
	encryptionKey string
	authURL       string
	tokenURL      string
	userInfoURL   string
	revokeURL     string
	gmailBaseURL  string
	store         TokenStore
	httpClient    *http.Client
	logger        *slog.Logger
}

// NewGmailService creates a GmailService.
func NewGmailService(cfg ServiceConfig, store TokenStore) *GmailService {
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://oauth2.googleapis.com/token"
	}
	if cfg.UserInfoURL == "" {
		cfg.UserInfoURL = "https://www.googleapis.com/oauth2/v3/userinfo"
	}
	if cfg.RevokeURL == "" {
		cfg.RevokeURL = "https://oauth2.googleapis.com/revoke"
	}
	if cfg.GmailBaseURL == "" {
		cfg.GmailBaseURL = "https://gmail.googleapis.com/gmail/v1/users/me"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &GmailService{
		clientID:      cfg.ClientID,
		clientSecret:  cfg.ClientSecret,
		redirectURL:   cfg.RedirectURL,
		encryptionKey: cfg.EncryptionKey,
		authURL:       cfg.AuthURL,
		tokenURL:      cfg.TokenURL,
		userInfoURL:   cfg.UserInfoURL,
		revokeURL:     cfg.RevokeURL,
		gmailBaseURL:  strings.TrimRight(cfg.GmailBaseURL, "/"),
		store:         store,
		httpClient:    cfg.HTTPClient,
		logger:        cfg.Logger,
	}
}

// GetAuthURL constructs the Google consent URL with offline access and required scopes.
func (s *GmailService) GetAuthURL(state string) string {
	values := url.Values{}
	values.Set("client_id", s.clientID)
	values.Set("redirect_uri", s.redirectURL)
	values.Set("response_type", "code")
	values.Set("scope", strings.Join(GoogleOAuthScopes, " "))
	values.Set("access_type", "offline")
	values.Set("prompt", "consent")
	if state != "" {
		values.Set("state", state)
	}

	return fmt.Sprintf("%s?%s", s.authURL, values.Encode())
}

type tokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type userInfoResponse struct {
	Email string `json:"email"`
}

// HandleCallback exchanges an authorization code for tokens, encrypts the refresh token, and saves it.
func (s *GmailService) HandleCallback(ctx context.Context, code string) (*OAuthTokenData, error) {
	if s.store == nil {
		return nil, errors.New("token store is not initialized")
	}

	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", s.clientID)
	data.Set("client_secret", s.clientSecret)
	data.Set("redirect_uri", s.redirectURL)
	data.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token exchange response: %w", err)
	}

	var tokenResp tokenExchangeResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token exchange response: %w", err)
	}

	if tokenResp.Error != "" || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google token exchange failed (%d): %s - %s", resp.StatusCode, tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Fetch user profile email
	userEmail, err := s.fetchUserEmail(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}

	// Encrypt refresh token
	var encryptedRefreshToken string
	if tokenResp.RefreshToken != "" {
		encrypted, err := EncryptAESGCM(tokenResp.RefreshToken, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
		}
		encryptedRefreshToken = encrypted
	} else {
		// Fallback to existing encrypted refresh token if not returned
		existing, _ := s.store.GetToken(ctx)
		if existing != nil {
			encryptedRefreshToken = existing.EncryptedRefreshToken
		}
	}

	expiry := time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	tokenData := &OAuthTokenData{
		AccountEmail:          userEmail,
		EncryptedRefreshToken: encryptedRefreshToken,
		AccessToken:           tokenResp.AccessToken,
		Expiry:                expiry,
		Scopes:                GoogleOAuthScopes,
		UpdatedAt:             time.Now().UTC(),
	}

	if err := s.store.SaveToken(ctx, tokenData); err != nil {
		return nil, fmt.Errorf("failed to persist oauth token: %w", err)
	}

	s.logger.Info("google oauth connection established successfully", "email", userEmail)
	return tokenData, nil
}

func (s *GmailService) fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.userInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("userinfo request returned status %d: %s", resp.StatusCode, string(b))
	}

	var user userInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", err
	}

	return user.Email, nil
}

// GetStatus checks whether an authorized Google token is active.
func (s *GmailService) GetStatus(ctx context.Context) (*AuthStatus, error) {
	if s.store == nil {
		return &AuthStatus{Connected: false}, nil
	}

	tokenData, err := s.store.GetToken(ctx)
	if err != nil || tokenData == nil || tokenData.AccountEmail == "" {
		return &AuthStatus{Connected: false}, nil
	}

	return &AuthStatus{
		Connected: true,
		Email:     tokenData.AccountEmail,
		Scopes:    tokenData.Scopes,
		UpdatedAt: &tokenData.UpdatedAt,
	}, nil
}

// Disconnect revokes credentials with Google and clears stored tokens.
func (s *GmailService) Disconnect(ctx context.Context) error {
	if s.store == nil {
		return nil
	}

	tokenData, err := s.store.GetToken(ctx)
	if err == nil && tokenData != nil {
		// Attempt token revocation with Google
		targetToken := tokenData.AccessToken
		if tokenData.EncryptedRefreshToken != "" {
			if decrypted, err := DecryptAESGCM(tokenData.EncryptedRefreshToken, s.encryptionKey); err == nil && decrypted != "" {
				targetToken = decrypted
			}
		}

		if targetToken != "" {
			revokeURL := fmt.Sprintf("%s?token=%s", s.revokeURL, url.QueryEscape(targetToken))
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, nil)
			if req != nil {
				_, _ = s.httpClient.Do(req)
			}
		}
	}

	return s.store.DeleteToken(ctx)
}

// getValidAccessToken returns an active access token, refreshing it if expired.
func (s *GmailService) getValidAccessToken(ctx context.Context) (string, string, error) {
	if s.store == nil {
		return "", "", errors.New("token store is not initialized")
	}

	tokenData, err := s.store.GetToken(ctx)
	if err != nil || tokenData == nil || tokenData.AccountEmail == "" {
		return "", "", errors.New("google account is not connected; please connect first")
	}

	// If access token is valid for at least another 60 seconds, use it
	if tokenData.AccessToken != "" && time.Now().UTC().Before(tokenData.Expiry.Add(-60*time.Second)) {
		return tokenData.AccessToken, tokenData.AccountEmail, nil
	}

	// Decrypt refresh token
	if tokenData.EncryptedRefreshToken == "" {
		return "", "", errors.New("no refresh token available to refresh expired access token")
	}

	refreshToken, err := DecryptAESGCM(tokenData.EncryptedRefreshToken, s.encryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	// Refresh access token
	data := url.Values{}
	data.Set("client_id", s.clientID)
	data.Set("client_secret", s.clientSecret)
	data.Set("refresh_token", refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("failed to create refresh token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("google refresh token failed (%d): %s", resp.StatusCode, string(b))
	}

	var tokenResp tokenExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", fmt.Errorf("failed to decode refresh token response: %w", err)
	}

	tokenData.AccessToken = tokenResp.AccessToken
	tokenData.Expiry = time.Now().UTC().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	tokenData.UpdatedAt = time.Now().UTC()

	_ = s.store.SaveToken(ctx, tokenData)

	return tokenData.AccessToken, tokenData.AccountEmail, nil
}

type sendMailPayload struct {
	Raw      string `json:"raw"`
	ThreadID string `json:"threadId,omitempty"`
}

type sendMailResponse struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

// SendMail builds a multipart/mixed MIME message and sends it via the Gmail REST API.
func (s *GmailService) SendMail(
	ctx context.Context,
	to, subject, body string,
	attachments []Attachment,
	inReplyTo, references, threadID string,
) (messageID, threadIDOut string, err error) {
	accessToken, _, err := s.getValidAccessToken(ctx)
	if err != nil {
		return "", "", err
	}

	rawBase64, err := BuildRawMIME(to, subject, body, attachments, inReplyTo, references)
	if err != nil {
		return "", "", fmt.Errorf("failed to build MIME payload: %w", err)
	}

	payload := sendMailPayload{
		Raw:      rawBase64,
		ThreadID: threadID,
	}

	reqBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal send mail payload: %w", err)
	}

	sendURL := fmt.Sprintf("%s/messages/send", s.gmailBaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(reqBytes))
	if err != nil {
		return "", "", fmt.Errorf("failed to create send message request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("gmail send message request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read send message response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("gmail API send failed (%d): %s", resp.StatusCode, string(respBytes))
	}

	var sendResp sendMailResponse
	if err := json.Unmarshal(respBytes, &sendResp); err != nil {
		return "", "", fmt.Errorf("failed to decode send response: %w", err)
	}

	s.logger.Info("email sent via gmail api",
		"to", to,
		"message_id", sendResp.ID,
		"thread_id", sendResp.ThreadID,
	)

	return sendResp.ID, sendResp.ThreadID, nil
}

type gmailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type gmailMessagePayload struct {
	Headers []gmailHeader `json:"headers"`
}

type gmailMessage struct {
	ID           string              `json:"id"`
	ThreadID     string              `json:"threadId"`
	Snippet      string              `json:"snippet"`
	InternalDate string              `json:"internalDate"`
	Payload      gmailMessagePayload `json:"payload"`
}

type gmailThread struct {
	ID       string         `json:"id"`
	Messages []gmailMessage `json:"messages"`
}

// CheckThreads inspects a list of thread IDs, extracting new inbound messages and detecting bounces.
func (s *GmailService) CheckThreads(ctx context.Context, threadIDs []string) ([]InboundMessage, error) {
	if len(threadIDs) == 0 {
		return nil, nil
	}

	accessToken, accountEmail, err := s.getValidAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var inboundMessages []InboundMessage

	for _, tid := range threadIDs {
		threadURL := fmt.Sprintf("%s/threads/%s?format=full", s.gmailBaseURL, tid)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, threadURL, nil)
		if err != nil {
			s.logger.Warn("failed to create get thread request", "thread_id", tid, "error", err)
			continue
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			s.logger.Warn("get thread request failed", "thread_id", tid, "error", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			s.logger.Warn("get thread returned non-200", "thread_id", tid, "status", resp.StatusCode)
			continue
		}

		var thread gmailThread
		err = json.NewDecoder(resp.Body).Decode(&thread)
		resp.Body.Close()
		if err != nil {
			s.logger.Warn("failed to decode thread response", "thread_id", tid, "error", err)
			continue
		}

		for _, msg := range thread.Messages {
			headers := make(map[string]string)
			for _, h := range msg.Payload.Headers {
				headers[strings.ToLower(h.Name)] = h.Value
			}

			from := headers["from"]
			to := headers["to"]
			subject := headers["subject"]

			// Check if message is inbound (not sent from my account)
			if accountEmail != "" && strings.Contains(strings.ToLower(from), strings.ToLower(accountEmail)) {
				continue
			}

			// Parse date
			var receivedAt time.Time
			if msg.InternalDate != "" {
				if millis, err := strconv.ParseInt(msg.InternalDate, 10, 64); err == nil {
					receivedAt = time.UnixMilli(millis).UTC()
				}
			}
			if receivedAt.IsZero() {
				if dateHeader, ok := headers["date"]; ok {
					if parsed, err := http.ParseTime(dateHeader); err == nil {
						receivedAt = parsed.UTC()
					}
				}
			}
			if receivedAt.IsZero() {
				receivedAt = time.Now().UTC()
			}

			isBounce, bounceReason := DetectBounce(from, subject, msg.Snippet, headers)

			inboundMessages = append(inboundMessages, InboundMessage{
				ThreadID:     tid,
				MessageID:    msg.ID,
				From:         from,
				To:           to,
				Subject:      subject,
				Snippet:      msg.Snippet,
				ReceivedAt:   receivedAt,
				IsBounce:     isBounce,
				BounceReason: bounceReason,
			})
		}
	}

	return inboundMessages, nil
}

// DetectBounce determines if an inbound email represents a delivery failure or bounce.
func DetectBounce(from, subject, snippet string, headers map[string]string) (bool, string) {
	fromLower := strings.ToLower(from)
	subjectLower := strings.ToLower(subject)
	snippetLower := strings.ToLower(snippet)

	bounceSenders := []string{
		"mailer-daemon",
		"postmaster",
		"mail delivery subsystem",
		"noreply-daemon",
		"bounce",
	}
	for _, sender := range bounceSenders {
		if strings.Contains(fromLower, sender) {
			return true, fmt.Sprintf("bounce sender detected: %s", from)
		}
	}

	bouncePhrases := []string{
		"delivery status notification (failure)",
		"undelivered mail returned to sender",
		"failure notice",
		"message not delivered",
		"delivery failure",
		"delivery failed",
		"returned mail: see transcript",
		"address not found",
		"recipient address rejected",
		"mailbox unavailable",
		"550 5.1.1",
	}

	for _, phrase := range bouncePhrases {
		if strings.Contains(subjectLower, phrase) {
			return true, fmt.Sprintf("bounce subject: %s", subject)
		}
		if strings.Contains(snippetLower, phrase) {
			return true, fmt.Sprintf("bounce snippet: %s", phrase)
		}
	}

	if val, ok := headers["action"]; ok && strings.EqualFold(val, "failed") {
		return true, "action header indicates delivery failure"
	}

	return false, ""
}
