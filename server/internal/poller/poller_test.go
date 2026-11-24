package poller

import (
	"context"
	"testing"
	"time"

	"emailauto-server/internal/gmail"
	"emailauto-server/internal/model"
)

type mockOutreachStoreForPoller struct {
	sentThreadIDs    []string
	outreachByThread map[string]*model.Outreach
	statusUpdates    map[string]model.OutreachStatus
	invalidContacts  map[string]string
	cancelledCount   int
	needingF1        []model.Outreach
	needingF2        []model.Outreach
	followupCounts   map[string]int
	createdOutreach  []model.Outreach
}

func newMockOutreachStoreForPoller() *mockOutreachStoreForPoller {
	return &mockOutreachStoreForPoller{
		outreachByThread: make(map[string]*model.Outreach),
		statusUpdates:    make(map[string]model.OutreachStatus),
		invalidContacts:  make(map[string]string),
		followupCounts:   make(map[string]int),
	}
}

func (m *mockOutreachStoreForPoller) GetSentThreadIDs(ctx context.Context, days int) ([]string, error) {
	return m.sentThreadIDs, nil
}

func (m *mockOutreachStoreForPoller) GetOutreachByThreadID(ctx context.Context, threadID string) (*model.Outreach, error) {
	return m.outreachByThread[threadID], nil
}

func (m *mockOutreachStoreForPoller) UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error {
	m.statusUpdates[id] = status
	return nil
}

func (m *mockOutreachStoreForPoller) CancelPendingFollowups(ctx context.Context, contactID string) error {
	m.cancelledCount++
	return nil
}

func (m *mockOutreachStoreForPoller) MarkContactInvalid(ctx context.Context, contactID string, reason string) error {
	m.invalidContacts[contactID] = reason
	return nil
}

func (m *mockOutreachStoreForPoller) GetSentOutreachNeedingFollowup1(ctx context.Context, afterHours int) ([]model.Outreach, error) {
	return m.needingF1, nil
}

func (m *mockOutreachStoreForPoller) GetSentOutreachNeedingFollowup2(ctx context.Context, afterHours int) ([]model.Outreach, error) {
	return m.needingF2, nil
}

func (m *mockOutreachStoreForPoller) CountFollowupsForContact(ctx context.Context, contactID string) (int, error) {
	return m.followupCounts[contactID], nil
}

func (m *mockOutreachStoreForPoller) CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error) {
	o.ID = "created_followup"
	m.createdOutreach = append(m.createdOutreach, *o)
	m.followupCounts[o.ContactID]++
	return o, nil
}

type mockReplyStoreForPoller struct {
	savedReplies []model.Reply
}

func (m *mockReplyStoreForPoller) SaveReply(ctx context.Context, reply *model.Reply) (*model.Reply, error) {
	m.savedReplies = append(m.savedReplies, *reply)
	return reply, nil
}

func (m *mockReplyStoreForPoller) HasReplyForOutreach(ctx context.Context, outreachID string) (bool, error) {
	return false, nil
}

type mockThreadChecker struct {
	messages []gmail.InboundMessage
}

func (m *mockThreadChecker) CheckThreads(ctx context.Context, threadIDs []string) ([]gmail.InboundMessage, error) {
	return m.messages, nil
}

type mockSettingsStoreForPoller struct {
	settings *model.Settings
}

func (m *mockSettingsStoreForPoller) Get(ctx context.Context) (*model.Settings, error) {
	return m.settings, nil
}

func TestPoller_ReplyReceived_MarksRepliedAndCancelsFollowups(t *testing.T) {
	ctx := context.Background()

	outreachStore := newMockOutreachStoreForPoller()
	outreachStore.sentThreadIDs = []string{"thread_abc"}
	outreachStore.outreachByThread["thread_abc"] = &model.Outreach{
		ID:        "o_initial",
		ContactID: "c_recruiter",
		Status:    model.OutreachStatusSent,
	}

	replyStore := &mockReplyStoreForPoller{}
	threadChecker := &mockThreadChecker{
		messages: []gmail.InboundMessage{
			{
				ThreadID:   "thread_abc",
				MessageID:  "inbound_msg_1",
				Snippet:    "Thanks for reaching out! Let's schedule a call.",
				ReceivedAt: time.Now().UTC(),
				IsBounce:   false,
			},
		},
	}

	p := NewPoller(30*time.Minute, outreachStore, replyStore, nil, threadChecker, nil)
	report, err := p.PollOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.RepliesRecorded != 1 {
		t.Errorf("expected 1 reply recorded, got %d", report.RepliesRecorded)
	}
	if outreachStore.statusUpdates["o_initial"] != model.OutreachStatusReplied {
		t.Errorf("expected outreach to be marked replied, got: %s", outreachStore.statusUpdates["o_initial"])
	}
	if outreachStore.cancelledCount != 1 {
		t.Errorf("expected followups to be cancelled, got cancelledCount = %d", outreachStore.cancelledCount)
	}
	if len(replyStore.savedReplies) != 1 {
		t.Errorf("expected 1 reply saved in DB, got %d", len(replyStore.savedReplies))
	}
}

func TestPoller_BounceDetected_MarksFailedAndInvalidatesContact(t *testing.T) {
	ctx := context.Background()

	outreachStore := newMockOutreachStoreForPoller()
	outreachStore.sentThreadIDs = []string{"thread_bounce"}
	outreachStore.outreachByThread["thread_bounce"] = &model.Outreach{
		ID:        "o_bounced",
		ContactID: "c_bad_email",
		Status:    model.OutreachStatusSent,
	}

	threadChecker := &mockThreadChecker{
		messages: []gmail.InboundMessage{
			{
				ThreadID:     "thread_bounce",
				MessageID:    "bounce_msg_1",
				IsBounce:     true,
				BounceReason: "550 5.1.1 User unknown",
			},
		},
	}

	p := NewPoller(30*time.Minute, outreachStore, &mockReplyStoreForPoller{}, nil, threadChecker, nil)
	report, err := p.PollOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.BouncesDetected != 1 {
		t.Errorf("expected 1 bounce detected, got %d", report.BouncesDetected)
	}
	if outreachStore.statusUpdates["o_bounced"] != model.OutreachStatusFailed {
		t.Errorf("expected outreach marked failed, got: %s", outreachStore.statusUpdates["o_bounced"])
	}
	if outreachStore.invalidContacts["c_bad_email"] == "" {
		t.Errorf("expected contact to be marked invalid on bounce")
	}
}

func TestPoller_FollowupDraftGeneration(t *testing.T) {
	ctx := context.Background()

	outreachStore := newMockOutreachStoreForPoller()
	outreachStore.needingF1 = []model.Outreach{
		{
			ID:             "o_sent_1",
			ContactID:      "c_needs_f1",
			Subject:        "Backend Developer Role",
			Role:           "Backend Developer",
			Company:        "Acme",
			GmailThreadID:  "thread_1",
			GmailMessageID: "msg_1",
			Kind:           model.OutreachKindInitial,
		},
	}
	outreachStore.needingF2 = []model.Outreach{
		{
			ID:             "o_f1_sent",
			ContactID:      "c_needs_f2",
			Subject:        "Re: Backend Developer Role",
			Role:           "Backend Developer",
			Company:        "Acme",
			GmailThreadID:  "thread_2",
			GmailMessageID: "msg_2",
			Kind:           model.OutreachKindFollowup1,
		},
	}
	outreachStore.followupCounts["c_needs_f1"] = 0
	outreachStore.followupCounts["c_needs_f2"] = 1 // 1 followup exists, eligible for final 2nd

	settingsStore := &mockSettingsStoreForPoller{
		settings: &model.Settings{
			FollowupAfterHours: 24,
			ApprovalMode:       true, // should stay as draft
		},
	}

	p := NewPoller(30*time.Minute, outreachStore, &mockReplyStoreForPoller{}, settingsStore, nil, nil)
	report, err := p.PollOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Followup1Drafted != 1 {
		t.Errorf("expected 1 followup1 drafted, got %d", report.Followup1Drafted)
	}
	if report.Followup2Drafted != 1 {
		t.Errorf("expected 1 followup2 drafted, got %d", report.Followup2Drafted)
	}
	if len(outreachStore.createdOutreach) != 2 {
		t.Fatalf("expected 2 created followups, got %d", len(outreachStore.createdOutreach))
	}
	// Verify created followups are drafts
	for _, f := range outreachStore.createdOutreach {
		if f.Status != model.OutreachStatusDraft {
			t.Errorf("expected followup status to be draft under approval_mode, got: %s", f.Status)
		}
	}
}

func TestPoller_MaxTwoFollowupsEver(t *testing.T) {
	ctx := context.Background()

	outreachStore := newMockOutreachStoreForPoller()
	outreachStore.needingF1 = []model.Outreach{
		{ID: "o_exhausted", ContactID: "c_maxed"},
	}
	// Already has 2 follow-ups
	outreachStore.followupCounts["c_maxed"] = 2

	p := NewPoller(30*time.Minute, outreachStore, nil, nil, nil, nil)
	report, err := p.PollOnce(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Followup1Drafted != 0 {
		t.Errorf("expected 0 followups drafted when contact already has 2, got %d", report.Followup1Drafted)
	}
}
