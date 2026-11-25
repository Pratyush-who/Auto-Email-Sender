package sender

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"emailauto-server/internal/gmail"
	"emailauto-server/internal/model"
)

type mockOutreachStore struct {
	pending          []model.Outreach
	sentTodayCount   int
	recentSentEmail  map[string]bool
	existingInitials map[string]bool
	statusUpdates    map[string]model.OutreachStatus
	invalidContacts  map[string]string
}

func newMockOutreachStore() *mockOutreachStore {
	return &mockOutreachStore{
		recentSentEmail:  make(map[string]bool),
		existingInitials: make(map[string]bool),
		statusUpdates:    make(map[string]model.OutreachStatus),
		invalidContacts:  make(map[string]string),
	}
}

func (m *mockOutreachStore) GetPendingOutreachToSend(ctx context.Context, limit int) ([]model.Outreach, error) {
	if len(m.pending) == 0 {
		return nil, nil
	}
	res := m.pending[:1]
	m.pending = m.pending[1:]
	return res, nil
}

func (m *mockOutreachStore) CountSentToday(ctx context.Context) (int, error) {
	return m.sentTodayCount, nil
}

func (m *mockOutreachStore) HasSentToAddressOrCompanyWithinDays(ctx context.Context, email, company string, days int) (bool, error) {
	return m.recentSentEmail[email] || m.recentSentEmail[company], nil
}

func (m *mockOutreachStore) HasExistingInitialOutreach(ctx context.Context, contactID string, excludeID ...string) (bool, error) {
	return m.existingInitials[contactID], nil
}

func (m *mockOutreachStore) UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error {
	m.statusUpdates[id] = status
	return nil
}

func (m *mockOutreachStore) MarkContactInvalid(ctx context.Context, contactID string, reason string) error {
	m.invalidContacts[contactID] = reason
	return nil
}

func (m *mockOutreachStore) ListOutreach(ctx context.Context, filter model.OutreachFilter) ([]model.Outreach, error) {
	return m.pending, nil
}

type mockProfileStore struct {
	profile *model.Profile
}

func (m *mockProfileStore) Get(ctx context.Context) (*model.Profile, error) {
	return m.profile, nil
}

type mockSettingsStore struct {
	settings *model.Settings
}

func (m *mockSettingsStore) Get(ctx context.Context) (*model.Settings, error) {
	return m.settings, nil
}

type mockEmailSender struct {
	sentTo          string
	lastAttachments []gmail.Attachment
	err             error
}

func (m *mockEmailSender) SendMail(ctx context.Context, to, subject, body string, attachments []gmail.Attachment, inReplyTo, references, threadID string) (string, string, error) {
	m.sentTo = to
	m.lastAttachments = attachments
	if m.err != nil {
		return "", "", m.err
	}
	return "msg_12345", "thread_12345", nil
}

func TestSendWindowEvaluation(t *testing.T) {
	ist := time.FixedZone("IST", 5*3600+30*60)

	// Case 1: 14:30 IST -> inside 09:00-19:00
	midday := time.Date(2026, 11, 24, 14, 30, 0, 0, ist)
	inWin, _ := IsWithinSendWindow(midday, "09:00-19:00 IST")
	if !inWin {
		t.Errorf("expected 14:30 IST to be within 09:00-19:00 window")
	}

	// Case 2: 23:00 IST -> outside 09:00-19:00
	night := time.Date(2026, 11, 24, 23, 0, 0, 0, ist)
	inWinNight, reason := IsWithinSendWindow(night, "09:00-19:00 IST")
	if inWinNight {
		t.Errorf("expected 23:00 IST to be outside send window, reason: %s", reason)
	}

	// Case 3: 07:00 IST -> outside window
	morning := time.Date(2026, 11, 24, 7, 0, 0, 0, ist)
	inWinMorning, _ := IsWithinSendWindow(morning, "09:00-19:00 IST")
	if inWinMorning {
		t.Errorf("expected 07:00 IST to be outside send window")
	}
}

func TestSender_DailyCapEnforced(t *testing.T) {
	ctx := context.Background()
	store := newMockOutreachStore()
	store.sentTodayCount = 30 // at daily cap

	settingsStore := &mockSettingsStore{
		settings: &model.Settings{
			DailyCap:   30,
			SendWindow: "00:00-23:59 UTC",
		},
	}

	s := NewSender(store, nil, settingsStore, nil, nil)
	s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

	res, err := s.SendNext(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "daily send cap") {
		t.Errorf("expected daily cap skip, got: %+v", res)
	}
}

func TestSender_DuplicateWithin30DaysSkipped(t *testing.T) {
	ctx := context.Background()
	store := newMockOutreachStore()
	store.pending = []model.Outreach{
		{
			ID:           "outreach_1",
			ContactEmail: "recruiter@acme.com",
			Company:      "Acme Corp",
			Kind:         model.OutreachKindInitial,
			Status:       model.OutreachStatusQueued,
		},
	}
	// Flag email as already emailed within 30 days
	store.recentSentEmail["recruiter@acme.com"] = true

	settingsStore := &mockSettingsStore{
		settings: &model.Settings{DailyCap: 30, SendWindow: "00:00-23:59 UTC"},
	}

	s := NewSender(store, nil, settingsStore, nil, nil)
	s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

	res, err := s.SendNext(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped || !strings.Contains(res.Reason, "30 days") {
		t.Errorf("expected 30 days duplicate skip, got: %+v", res)
	}
	if store.statusUpdates["outreach_1"] != model.OutreachStatusSkipped {
		t.Errorf("expected outreach_1 status to be updated to skipped")
	}
}

func TestSender_SuccessfulSendWithAttachment(t *testing.T) {
	ctx := context.Background()
	store := newMockOutreachStore()
	store.pending = []model.Outreach{
		{
			ID:           "outreach_valid",
			ContactID:    "contact_1",
			ContactEmail: "lead@google.com",
			Company:      "Google",
			Subject:      "Application for Go Role",
			Body:         "Hello lead, please find attached my resume.",
			Kind:         model.OutreachKindInitial,
			Status:       model.OutreachStatusQueued,
		},
	}

	profileStore := &mockProfileStore{
		profile: &model.Profile{
			ResumePath: "uploads/resume_pratyush.pdf",
		},
	}
	settingsStore := &mockSettingsStore{
		settings: &model.Settings{
			DailyCap:      30,
			SendWindow:    "00:00-23:59 UTC",
			GapMinMinutes: 1,
			GapMaxMinutes: 2,
		},
	}
	mailer := &mockEmailSender{}

	s := NewSender(store, profileStore, settingsStore, mailer, nil)
	s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }
	s.sleepFunc = func(d time.Duration) {} // No sleep during tests
	s.readFile = func(path string) ([]byte, error) {
		return []byte("%PDF-1.4 sample resume content"), nil
	}

	res, err := s.SendNext(ctx, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Sent {
		t.Fatalf("expected email to be sent successfully, got: %+v", res)
	}
	if mailer.sentTo != "lead@google.com" {
		t.Errorf("expected send to lead@google.com, got %s", mailer.sentTo)
	}
	if len(mailer.lastAttachments) != 1 {
		t.Fatalf("expected 1 resume attachment, got %d", len(mailer.lastAttachments))
	}
	if mailer.lastAttachments[0].Filename != "resume_pratyush.pdf" {
		t.Errorf("unexpected attachment filename: %s", mailer.lastAttachments[0].Filename)
	}
	if store.statusUpdates["outreach_valid"] != model.OutreachStatusSent {
		t.Errorf("expected status sent, got %s", store.statusUpdates["outreach_valid"])
	}
}

func TestSender_GmailErrors_429And401AndBounce(t *testing.T) {
	ctx := context.Background()
	settingsStore := &mockSettingsStore{
		settings: &model.Settings{DailyCap: 30, SendWindow: "00:00-23:59 UTC"},
	}

	// 1. Test 429 Quota Exceeded -> Pauses till tomorrow
	{
		store := newMockOutreachStore()
		store.pending = []model.Outreach{
			{ID: "o_429", ContactEmail: "hr@company.com", Status: model.OutreachStatusQueued},
		}
		mailer := &mockEmailSender{err: errors.New("gmail API returned 429: rateLimitExceeded quota exceeded")}

		s := NewSender(store, nil, settingsStore, mailer, nil)
		s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

		res, _ := s.SendNext(ctx, false)
		if res.Sent {
			t.Errorf("expected 429 to not be sent")
		}
		status, _ := s.GetStatus(ctx)
		if status.State != SenderStatusPaused {
			t.Errorf("expected sender to be paused on 429, got: %s", status.State)
		}
		if status.PausedUntil == nil {
			t.Errorf("expected paused until timestamp for tomorrow")
		}
	}

	// 2. Test 401 Unauthorized -> Pauses and prompts reconnect
	{
		store := newMockOutreachStore()
		store.pending = []model.Outreach{
			{ID: "o_401", ContactEmail: "hr@company.com", Status: model.OutreachStatusQueued},
		}
		mailer := &mockEmailSender{err: errors.New("gmail API error (401): invalid_grant token expired")}

		s := NewSender(store, nil, settingsStore, mailer, nil)
		s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

		_, _ = s.SendNext(ctx, false)
		status, _ := s.GetStatus(ctx)
		if status.State != SenderStatusPaused {
			t.Errorf("expected sender to be paused on 401, got: %s", status.State)
		}
		if !strings.Contains(status.PauseReason, "reconnect") {
			t.Errorf("expected reconnect prompt in pause reason, got: %s", status.PauseReason)
		}
	}

	// 3. Test Bounce -> Marks contact invalid
	{
		store := newMockOutreachStore()
		store.pending = []model.Outreach{
			{ID: "o_bounce", ContactID: "c_bad", ContactEmail: "fake@invalid.com", Status: model.OutreachStatusQueued},
		}
		mailer := &mockEmailSender{err: errors.New("550 5.1.1 mailbox unavailable recipient rejected bounce")}

		s := NewSender(store, nil, settingsStore, mailer, nil)
		s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC) }

		_, _ = s.SendNext(ctx, false)
		if store.statusUpdates["o_bounce"] != model.OutreachStatusFailed {
			t.Errorf("expected outreach to be marked failed on bounce")
		}
		if store.invalidContacts["c_bad"] == "" {
			t.Errorf("expected contact to be marked invalid on bounce")
		}
	}
}

func TestSender_ForceBypassesWindow(t *testing.T) {
	ctx := context.Background()
	store := newMockOutreachStore()
	store.pending = []model.Outreach{
		{ID: "o_force", ContactEmail: "hr@company.com", Status: model.OutreachStatusQueued},
	}
	mailer := &mockEmailSender{}
	settingsStore := &mockSettingsStore{
		settings: &model.Settings{DailyCap: 30, SendWindow: "09:00-19:00 IST"},
	}
	s := NewSender(store, nil, settingsStore, mailer, nil)
	s.sleepFunc = func(d time.Duration) {}
	ist := time.FixedZone("IST", 5*3600+30*60)
	s.nowFunc = func() time.Time { return time.Date(2026, 11, 24, 0, 30, 0, 0, ist) }

	// Without force -> skipped outside window
	res1, _ := s.SendNext(ctx, false)
	if !res1.Skipped || !strings.Contains(res1.Reason, "send window") {
		t.Errorf("expected skip outside window without force, got %+v", res1)
	}

	// With force -> sends!
	res2, err := s.SendNext(ctx, true)
	if err != nil || !res2.Sent {
		t.Errorf("expected send with force=true, got %+v, err: %v", res2, err)
	}
}

func TestSender_Controls(t *testing.T) {
	ctx := context.Background()
	s := NewSender(newMockOutreachStore(), nil, nil, nil, nil)

	// Pause
	s.Pause("manual test pause", nil)
	st, _ := s.GetStatus(ctx)
	if st.State != SenderStatusPaused || st.PauseReason != "manual test pause" {
		t.Errorf("unexpected paused status: %+v", st)
	}

	// Resume
	s.Resume()
	st, _ = s.GetStatus(ctx)
	if st.State != SenderStatusIdle {
		t.Errorf("unexpected resumed status: %+v", st)
	}

	// StopAll
	_ = s.StopAll(ctx)
	st, _ = s.GetStatus(ctx)
	if st.State != SenderStatusStopped {
		t.Errorf("unexpected stopped status: %+v", st)
	}
}
