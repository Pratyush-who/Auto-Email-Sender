package model

import "time"

// Contact represents an extracted lead/contact stored in the contacts table.
type Contact struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Company      string    `json:"company"`
	Domain       string    `json:"domain"`
	TrustScore   float64   `json:"trust_score"`
	TrustReasons []string  `json:"trust_reasons"`
	PostID       string    `json:"post_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
