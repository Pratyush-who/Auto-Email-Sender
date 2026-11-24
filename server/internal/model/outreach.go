package model

import "time"

// OutreachKind represents the stage of outreach.
type OutreachKind string

const (
	OutreachKindInitial   OutreachKind = "initial"
	OutreachKindFollowup1 OutreachKind = "followup1"
	OutreachKindFollowup2 OutreachKind = "followup2"
)

// OutreachStatus represents the lifecycle state of an email outreach.
type OutreachStatus string

const (
	OutreachStatusDraft    OutreachStatus = "draft"
	OutreachStatusApproved OutreachStatus = "approved"
	OutreachStatusQueued   OutreachStatus = "queued"
	OutreachStatusSent     OutreachStatus = "sent"
	OutreachStatusFailed   OutreachStatus = "failed"
	OutreachStatusSkipped  OutreachStatus = "skipped"
	OutreachStatusReplied  OutreachStatus = "replied"
)

// Outreach represents a record in the outreach table.
type Outreach struct {
	ID             string         `json:"id"`
	ContactID      string         `json:"contact_id"`
	PostID         string         `json:"post_id,omitempty"`
	Kind           OutreachKind   `json:"kind"`
	Subject        string         `json:"subject"`
	Body           string         `json:"body"`
	Status         OutreachStatus `json:"status"`
	GmailMessageID string         `json:"gmail_message_id,omitempty"`
	GmailThreadID  string         `json:"gmail_thread_id,omitempty"`
	ScheduledAt    *time.Time     `json:"scheduled_at,omitempty"`
	SentAt         *time.Time     `json:"sent_at,omitempty"`
	Error          string         `json:"error,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`

	// Joined metadata fields for UI display
	ContactEmail string `json:"contact_email,omitempty"`
	Company      string `json:"company,omitempty"`
	Role         string `json:"role,omitempty"`
}

// OutreachFilter specifies query criteria for listing outreach items.
type OutreachFilter struct {
	Status OutreachStatus
	Kind   OutreachKind
	Limit  int
	Offset int
}
