package gmail

import (
	"context"
	"time"
)

// Scopes required for Gmail and user profile.
var GoogleOAuthScopes = []string{
	"https://www.googleapis.com/auth/gmail.send",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/userinfo.email",
}

// OAuthTokenData contains stored tokens and connection metadata.
type OAuthTokenData struct {
	AccountEmail          string    `json:"account_email"`
	EncryptedRefreshToken string    `json:"encrypted_refresh_token"`
	AccessToken           string    `json:"access_token"`
	Expiry                time.Time `json:"expiry"`
	Scopes                []string  `json:"scopes"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// AuthStatus represents the public connection status of the Google account.
type AuthStatus struct {
	Connected bool       `json:"connected"`
	Email     string     `json:"email"`
	Scopes    []string   `json:"scopes"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// InboundMessage represents an incoming message in a monitored thread.
type InboundMessage struct {
	ThreadID     string    `json:"thread_id"`
	MessageID    string    `json:"message_id"`
	From         string    `json:"from"`
	To           string    `json:"to"`
	Subject      string    `json:"subject"`
	Snippet      string    `json:"snippet"`
	ReceivedAt   time.Time `json:"received_at"`
	IsBounce     bool      `json:"is_bounce"`
	BounceReason string    `json:"bounce_reason,omitempty"`
}

// TokenStore is the storage abstraction for persisting OAuth tokens.
type TokenStore interface {
	SaveToken(ctx context.Context, data *OAuthTokenData) error
	GetToken(ctx context.Context) (*OAuthTokenData, error)
	DeleteToken(ctx context.Context) error
}
