package sender

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"emailauto-server/internal/gmail"
	"emailauto-server/internal/model"
)

// SenderStatus represents the operational state of the email sender.
type SenderStatus string

const (
	SenderStatusIdle    SenderStatus = "idle"
	SenderStatusRunning SenderStatus = "running"
	SenderStatusPaused  SenderStatus = "paused"
	SenderStatusStopped SenderStatus = "stopped"
)

// OutreachStore defines database operations needed by the sender.
type OutreachStore interface {
	GetPendingOutreachToSend(ctx context.Context, limit int) ([]model.Outreach, error)
	CountSentToday(ctx context.Context) (int, error)
	HasSentToAddressOrCompanyWithinDays(ctx context.Context, email, company string, days int) (bool, error)
	HasExistingInitialOutreach(ctx context.Context, contactID string) (bool, error)
	UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error
	MarkContactInvalid(ctx context.Context, contactID string, reason string) error
	ListOutreach(ctx context.Context, filter model.OutreachFilter) ([]model.Outreach, error)
}

// ProfileStore defines profile access for resume attachments.
type ProfileStore interface {
	Get(ctx context.Context) (*model.Profile, error)
}

// SettingsStore defines access to scheduling and rate limit settings.
type SettingsStore interface {
	Get(ctx context.Context) (*model.Settings, error)
}

// EmailSender defines the method for sending emails via Gmail.
type EmailSender interface {
	SendMail(ctx context.Context, to, subject, body string, attachments []gmail.Attachment, inReplyTo, references, threadID string) (messageID, threadIDOut string, err error)
}

// StatusReport provides a snapshot of sender state and metrics.
type StatusReport struct {
	State       SenderStatus `json:"state"`
	PauseReason string       `json:"pause_reason,omitempty"`
	PausedUntil *time.Time   `json:"paused_until,omitempty"`
	DailySent   int          `json:"daily_sent"`
	DailyCap    int          `json:"daily_cap"`
	ActiveQueue int          `json:"active_queue"`
}

// SendResult describes the outcome of a single outreach attempt.
type SendResult struct {
	OutreachID   string `json:"outreach_id"`
	Sent         bool   `json:"sent"`
	Skipped      bool   `json:"skipped"`
	Reason       string `json:"reason,omitempty"`
	GmailMessage string `json:"gmail_message_id,omitempty"`
}

// Sender orchestrates queue processing, guardrail enforcement, and dispatching.
type Sender struct {
	mu            sync.RWMutex
	state         SenderStatus
	pauseReason   string
	pausedUntil   *time.Time
	outreachStore OutreachStore
	profileStore  ProfileStore
	settingsStore SettingsStore
	mailer        EmailSender
	logger        *slog.Logger

	// Testable hooks
	nowFunc     func() time.Time
	sleepFunc   func(d time.Duration)
	readFile    func(path string) ([]byte, error)
	randIntn    func(n int) int
}

// NewSender creates a new Sender instance.
func NewSender(
	outreachStore OutreachStore,
	profileStore ProfileStore,
	settingsStore SettingsStore,
	mailer EmailSender,
	logger *slog.Logger,
) *Sender {
	if logger == nil {
		logger = slog.Default()
	}
	return &Sender{
		state:         SenderStatusIdle,
		outreachStore: outreachStore,
		profileStore:  profileStore,
		settingsStore: settingsStore,
		mailer:        mailer,
		logger:        logger,
		nowFunc:       time.Now,
		sleepFunc:     time.Sleep,
		readFile:      os.ReadFile,
		randIntn:      rand.Intn,
	}
}

// Pause pauses the sender with an explanatory reason.
func (s *Sender) Pause(reason string, until *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = SenderStatusPaused
	s.pauseReason = reason
	s.pausedUntil = until
	s.logger.Info("sender paused", "reason", reason, "until", until)
}

// Resume clears paused or stopped state and resumes the sender.
func (s *Sender) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = SenderStatusIdle
	s.pauseReason = ""
	s.pausedUntil = nil
	s.logger.Info("sender resumed to idle/ready state")
}

// StopAll stops the sender and prevents further automatic dispatches.
func (s *Sender) StopAll(ctx context.Context) error {
	s.mu.Lock()
	s.state = SenderStatusStopped
	s.pauseReason = "stopped by user"
	s.pausedUntil = nil
	s.mu.Unlock()

	s.logger.Info("sender stop-all requested")
	return nil
}

// GetStatus returns the current status, metrics, and queue size.
func (s *Sender) GetStatus(ctx context.Context) (*StatusReport, error) {
	s.mu.RLock()
	state := s.state
	pauseReason := s.pauseReason
	pausedUntil := s.pausedUntil
	s.mu.RUnlock()

	// Check if pause expired
	if state == SenderStatusPaused && pausedUntil != nil && s.nowFunc().After(*pausedUntil) {
		s.Resume()
		state = SenderStatusIdle
		pauseReason = ""
		pausedUntil = nil
	}

	dailySent := 0
	if s.outreachStore != nil {
		if count, err := s.outreachStore.CountSentToday(ctx); err == nil {
			dailySent = count
		}
	}

	dailyCap := 30
	if s.settingsStore != nil {
		if settings, err := s.settingsStore.Get(ctx); err == nil && settings != nil {
			dailyCap = settings.DailyCap
		}
	}

	activeQueue := 0
	if s.outreachStore != nil {
		queued, _ := s.outreachStore.ListOutreach(ctx, model.OutreachFilter{
			Status: model.OutreachStatusQueued,
			Limit:  1000,
		})
		activeQueue = len(queued)
	}

	return &StatusReport{
		State:       state,
		PauseReason: pauseReason,
		PausedUntil: pausedUntil,
		DailySent:   dailySent,
		DailyCap:    dailyCap,
		ActiveQueue: activeQueue,
	}, nil
}

// SendNext picks the next approved/queued outreach and attempts to send it while enforcing all guardrails.
func (s *Sender) SendNext(ctx context.Context) (*SendResult, error) {
	s.mu.Lock()
	if s.state == SenderStatusPaused {
		// Check if pausedUntil expired
		if s.pausedUntil != nil && s.nowFunc().After(*s.pausedUntil) {
			s.state = SenderStatusIdle
			s.pauseReason = ""
			s.pausedUntil = nil
		} else {
			reason := s.pauseReason
			s.mu.Unlock()
			return &SendResult{Skipped: true, Reason: "sender is paused: " + reason}, nil
		}
	}
	if s.state == SenderStatusStopped {
		s.mu.Unlock()
		return &SendResult{Skipped: true, Reason: "sender is stopped"}, nil
	}
	s.mu.Unlock()

	// 1. Load active settings
	var settings model.Settings
	if s.settingsStore != nil {
		if st, err := s.settingsStore.Get(ctx); err == nil && st != nil {
			settings = *st
		} else {
			settings = model.DefaultSettings()
		}
	} else {
		settings = model.DefaultSettings()
	}

	now := s.nowFunc()

	// 2. Enforce send_window (default 09:00-19:00 IST)
	inWindow, winReason := IsWithinSendWindow(now, settings.SendWindow)
	if !inWindow {
		s.logger.Info("send skipped: outside send window", "reason", winReason)
		return &SendResult{Skipped: true, Reason: winReason}, nil
	}

	// 3. Enforce daily_cap (default 30)
	sentToday, err := s.outreachStore.CountSentToday(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to count sent today: %w", err)
	}
	if sentToday >= settings.DailyCap {
		s.logger.Info("send skipped: daily cap reached", "sent", sentToday, "cap", settings.DailyCap)
		return &SendResult{
			Skipped: true,
			Reason:  fmt.Sprintf("daily send cap (%d/%d) reached for today", sentToday, settings.DailyCap),
		}, nil
	}

	// 4. Fetch next pending outreach (approved or queued)
	pending, err := s.outreachStore.GetPendingOutreachToSend(ctx, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending outreach: %w", err)
	}
	if len(pending) == 0 {
		return &SendResult{Skipped: true, Reason: "no approved or queued outreach waiting in queue"}, nil
	}

	target := pending[0]

	// 5. Guardrail: Never send initial if one exists/sent for this contact
	if target.Kind == model.OutreachKindInitial {
		hasExisting, err := s.outreachStore.HasExistingInitialOutreach(ctx, target.ContactID)
		if err == nil && hasExisting {
			reason := "initial outreach already exists or was sent for this contact"
			s.logger.Warn("skipping outreach", "outreach_id", target.ID, "reason", reason)
			_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusSkipped, "", "", reason, nil)
			return &SendResult{OutreachID: target.ID, Skipped: true, Reason: reason}, nil
		}
	}

	// 6. Guardrail: Never email the same address or same company twice within 30 days
	hasRecent, err := s.outreachStore.HasSentToAddressOrCompanyWithinDays(ctx, target.ContactEmail, target.Company, 30)
	if err == nil && hasRecent {
		reason := fmt.Sprintf("email (%s) or company (%s) already contacted within the last 30 days", target.ContactEmail, target.Company)
		s.logger.Warn("skipping outreach", "outreach_id", target.ID, "reason", reason)
		_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusSkipped, "", "", reason, nil)
		return &SendResult{OutreachID: target.ID, Skipped: true, Reason: reason}, nil
	}

	// 7. Attach candidate resume PDF if present
	var attachments []gmail.Attachment
	if s.profileStore != nil {
		profile, err := s.profileStore.Get(ctx)
		if err == nil && profile != nil && strings.TrimSpace(profile.ResumePath) != "" {
			resumePath := strings.TrimSpace(profile.ResumePath)
			data, err := s.readFile(resumePath)
			if err == nil && len(data) > 0 {
				filename := filepath.Base(resumePath)
				if !strings.HasSuffix(strings.ToLower(filename), ".pdf") {
					filename += ".pdf"
				}
				attachments = append(attachments, gmail.Attachment{
					Filename:    filename,
					ContentType: "application/pdf",
					Content:     data,
				})
				s.logger.Info("attached resume to outreach", "path", resumePath, "bytes", len(data))
			} else {
				s.logger.Warn("could not read resume file for attachment", "path", resumePath, "error", err)
			}
		}
	}

	// 8. Dispatch email via Gmail
	if s.mailer == nil {
		return nil, fmt.Errorf("mailer service is not initialized")
	}

	msgID, threadID, sendErr := s.mailer.SendMail(
		ctx,
		target.ContactEmail,
		target.Subject,
		target.Body,
		attachments,
		"", "", "",
	)

	// 9. Error handling for Gmail response
	if sendErr != nil {
		s.logger.Error("failed to send email via gmail", "outreach_id", target.ID, "error", sendErr)
		errStr := sendErr.Error()
		errLower := strings.ToLower(errStr)

		// 429 / Quota Exceeded -> Pause till next day
		if strings.Contains(errLower, "429") || strings.Contains(errLower, "quota") || strings.Contains(errLower, "ratelimit") {
			nextDay := tomorrowMidnightIST(now)
			s.Pause("Gmail quota exceeded (429), paused until tomorrow", &nextDay)
			_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusQueued, "", "", "Gmail quota exceeded (429): paused until tomorrow", nil)
			return &SendResult{
				OutreachID: target.ID,
				Sent:       false,
				Reason:     "Gmail API 429 quota exceeded: sender paused until tomorrow",
			}, nil
		}

		// 401 Unauthorized -> Pause + Prompt Reconnect
		if strings.Contains(errLower, "401") || strings.Contains(errLower, "unauthorized") || strings.Contains(errLower, "invalid_grant") {
			s.Pause("Google authentication expired (401), please reconnect your account in settings", nil)
			_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusQueued, "", "", "Google auth expired (401)", nil)
			return &SendResult{
				OutreachID: target.ID,
				Sent:       false,
				Reason:     "Google authentication expired (401): reconnect required",
			}, nil
		}

		// Bounce / Delivery failure -> Mark contact invalid
		if strings.Contains(errLower, "bounce") || strings.Contains(errLower, "550") || strings.Contains(errLower, "mailbox unavailable") {
			_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusFailed, "", "", "Delivery failed: "+errStr, nil)
			_ = s.outreachStore.MarkContactInvalid(ctx, target.ContactID, errStr)
			return &SendResult{
				OutreachID: target.ID,
				Sent:       false,
				Reason:     "Email bounced/rejected: contact marked invalid",
			}, nil
		}

		// General send error
		_ = s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusFailed, "", "", errStr, nil)
		return &SendResult{
			OutreachID: target.ID,
			Sent:       false,
			Reason:     "Send failed: " + errStr,
		}, nil
	}

	// 10. Log send in outreach
	sentAt := s.nowFunc().UTC()
	if err := s.outreachStore.UpdateStatus(ctx, target.ID, model.OutreachStatusSent, msgID, threadID, "", &sentAt); err != nil {
		s.logger.Warn("failed to update outreach status to sent", "outreach_id", target.ID, "error", err)
	}

	s.logger.Info("outreach successfully dispatched",
		"outreach_id", target.ID,
		"to", target.ContactEmail,
		"msg_id", msgID,
		"thread_id", threadID,
	)

	// 11. Randomized gap delay (gap_min..gap_max minutes)
	gapMin := settings.GapMinMinutes
	if gapMin <= 0 {
		gapMin = 2
	}
	gapMax := settings.GapMaxMinutes
	if gapMax < gapMin {
		gapMax = gapMin + 4
	}

	gapMinutes := gapMin
	if gapMax > gapMin {
		gapMinutes += s.randIntn(gapMax - gapMin + 1)
	}
	gapDuration := time.Duration(gapMinutes) * time.Minute
	s.logger.Info("enforcing randomized pacing gap before next send", "minutes", gapMinutes)
	s.sleepFunc(gapDuration)

	return &SendResult{
		OutreachID:   target.ID,
		Sent:         true,
		GmailMessage: msgID,
	}, nil
}

// IsWithinSendWindow checks if current time falls within configured window (e.g. "09:00-19:00 IST").
func IsWithinSendWindow(now time.Time, windowStr string) (bool, string) {
	windowStr = strings.TrimSpace(windowStr)
	if windowStr == "" {
		windowStr = "09:00-19:00 IST"
	}

	parts := strings.Fields(windowStr)
	rangeStr := parts[0]
	zoneStr := "IST"
	if len(parts) > 1 {
		zoneStr = strings.ToUpper(parts[1])
	}

	// Determine timezone
	var loc *time.Location
	switch zoneStr {
	case "IST":
		loc = time.FixedZone("IST", 5*3600+30*60)
	case "UTC", "GMT":
		loc = time.UTC
	default:
		var err error
		loc, err = time.LoadLocation(zoneStr)
		if err != nil {
			loc = time.FixedZone("IST", 5*3600+30*60)
		}
	}

	localNow := now.In(loc)

	times := strings.Split(rangeStr, "-")
	if len(times) != 2 {
		return true, "" // Malformed format, allow send
	}

	startMins, err1 := parseMins(times[0])
	endMins, err2 := parseMins(times[1])
	if err1 != nil || err2 != nil {
		return true, ""
	}

	currentMins := localNow.Hour()*60 + localNow.Minute()
	if currentMins < startMins || currentMins > endMins {
		return false, fmt.Sprintf("current time (%02d:%02d %s) is outside send window (%s)",
			localNow.Hour(), localNow.Minute(), zoneStr, windowStr)
	}

	return true, ""
}

func parseMins(tStr string) (int, error) {
	parts := strings.Split(strings.TrimSpace(tStr), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time format")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return h*60 + m, nil
}

func tomorrowMidnightIST(t time.Time) time.Time {
	ist := time.FixedZone("IST", 5*3600+30*60)
	inIST := t.In(ist)
	tomorrow := time.Date(inIST.Year(), inIST.Month(), inIST.Day()+1, 0, 0, 0, 0, ist)
	return tomorrow.UTC()
}
