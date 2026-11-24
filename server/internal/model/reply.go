package model

import "time"

// Reply represents an inbound message received in response to an outreach email.
type Reply struct {
	ID             string    `json:"id"`
	OutreachID     string    `json:"outreach_id"`
	GmailMessageID string    `json:"gmail_message_id"`
	Snippet        string    `json:"snippet"`
	ReceivedAt     time.Time `json:"received_at"`
	CreatedAt      time.Time `json:"created_at"`

	// Joined metadata for UI display
	ContactEmail string `json:"contact_email,omitempty"`
	Company      string `json:"company,omitempty"`
	Subject      string `json:"subject,omitempty"`
	IsBounce     bool   `json:"is_bounce,omitempty"`
}
