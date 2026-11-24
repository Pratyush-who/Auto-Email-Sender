package poller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"emailauto-server/internal/gmail"
	"emailauto-server/internal/model"
)

// OutreachStore defines database operations needed by the Poller.
type OutreachStore interface {
	GetSentThreadIDs(ctx context.Context, days int) ([]string, error)
	GetOutreachByThreadID(ctx context.Context, threadID string) (*model.Outreach, error)
	UpdateStatus(ctx context.Context, id string, status model.OutreachStatus, gmailMsgID, gmailThreadID, errText string, sentAt *time.Time) error
	CancelPendingFollowups(ctx context.Context, contactID string) error
	MarkContactInvalid(ctx context.Context, contactID string, reason string) error
	GetSentOutreachNeedingFollowup1(ctx context.Context, afterHours int) ([]model.Outreach, error)
	GetSentOutreachNeedingFollowup2(ctx context.Context, afterHours int) ([]model.Outreach, error)
	CountFollowupsForContact(ctx context.Context, contactID string) (int, error)
	CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error)
}

// ReplyStore defines operations for persisting inbound replies.
type ReplyStore interface {
	SaveReply(ctx context.Context, reply *model.Reply) (*model.Reply, error)
	HasReplyForOutreach(ctx context.Context, outreachID string) (bool, error)
}

// SettingsStore defines access to settings for follow-up intervals and approval mode.
type SettingsStore interface {
	Get(ctx context.Context) (*model.Settings, error)
}

// ThreadChecker defines Gmail thread inspection capabilities.
type ThreadChecker interface {
	CheckThreads(ctx context.Context, threadIDs []string) ([]gmail.InboundMessage, error)
}

// PollReport summarizes the outcome of a poll cycle.
type PollReport struct {
	ThreadsChecked    int `json:"threads_checked"`
	RepliesRecorded   int `json:"replies_recorded"`
	BouncesDetected   int `json:"bounces_detected"`
	Followup1Drafted  int `json:"followup1_drafted"`
	Followup2Drafted  int `json:"followup2_drafted"`
}

// Poller runs periodic thread checks and generates follow-up drafts.
type Poller struct {
	mu            sync.Mutex
	interval      time.Duration
	stopCh        chan struct{}
	running       bool
	outreachStore OutreachStore
	replyStore    ReplyStore
	settingsStore SettingsStore
	threadChecker ThreadChecker
	logger        *slog.Logger
}

// NewPoller creates a new Poller instance.
func NewPoller(
	interval time.Duration,
	outreachStore OutreachStore,
	replyStore ReplyStore,
	settingsStore SettingsStore,
	threadChecker ThreadChecker,
	logger *slog.Logger,
) *Poller {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &Poller{
		interval:      interval,
		stopCh:        make(chan struct{}),
		outreachStore: outreachStore,
		replyStore:    replyStore,
		settingsStore: settingsStore,
		threadChecker: threadChecker,
		logger:        logger,
	}
}

// Start launches the background polling ticker (every 30 minutes while server is up).
func (p *Poller) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.stopCh = make(chan struct{})
	p.mu.Unlock()

	p.logger.Info("starting 30-minute gmail thread poller", "interval", p.interval)

	go func() {
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.stopCh:
				p.logger.Info("gmail thread poller stopped")
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				report, err := p.PollOnce(ctx)
				cancel()
				if err != nil {
					p.logger.Error("error during thread poll cycle", "error", err)
				} else {
					p.logger.Info("poll cycle completed",
						"threads_checked", report.ThreadsChecked,
						"replies_recorded", report.RepliesRecorded,
						"bounces", report.BouncesDetected,
						"followup1_drafted", report.Followup1Drafted,
						"followup2_drafted", report.Followup2Drafted,
					)
				}
			}
		}
	}()
}

// Stop shuts down the background poller ticker.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return
	}
	close(p.stopCh)
	p.running = false
}

// PollOnce executes a single thread check cycle and evaluates follow-up draft generation.
func (p *Poller) PollOnce(ctx context.Context) (*PollReport, error) {
	report := &PollReport{}

	// Load settings
	var settings model.Settings
	if p.settingsStore != nil {
		if s, err := p.settingsStore.Get(ctx); err == nil && s != nil {
			settings = *s
		} else {
			settings = model.DefaultSettings()
		}
	} else {
		settings = model.DefaultSettings()
	}

	// 1. Thread checking via Gmail
	if p.threadChecker != nil && p.outreachStore != nil {
		threadIDs, err := p.outreachStore.GetSentThreadIDs(ctx, 30)
		if err == nil && len(threadIDs) > 0 {
			report.ThreadsChecked = len(threadIDs)
			inbound, checkErr := p.threadChecker.CheckThreads(ctx, threadIDs)
			if checkErr != nil {
				p.logger.Warn("thread check returned error", "error", checkErr)
			} else {
				for _, msg := range inbound {
					parent, err := p.outreachStore.GetOutreachByThreadID(ctx, msg.ThreadID)
					if err != nil || parent == nil {
						continue
					}

					if msg.IsBounce {
						report.BouncesDetected++
						p.logger.Warn("bounce detected for outreach", "outreach_id", parent.ID, "reason", msg.BounceReason)
						_ = p.outreachStore.UpdateStatus(ctx, parent.ID, model.OutreachStatusFailed, "", "", "Bounce: "+msg.BounceReason, nil)
						_ = p.outreachStore.MarkContactInvalid(ctx, parent.ContactID, msg.BounceReason)
						_ = p.outreachStore.CancelPendingFollowups(ctx, parent.ContactID)
						continue
					}

					// Legitimate inbound reply
					if p.replyStore != nil {
						hasReply, _ := p.replyStore.HasReplyForOutreach(ctx, parent.ID)
						if !hasReply {
							_, saveErr := p.replyStore.SaveReply(ctx, &model.Reply{
								OutreachID:     parent.ID,
								GmailMessageID: msg.MessageID,
								Snippet:        msg.Snippet,
								ReceivedAt:     msg.ReceivedAt,
							})
							if saveErr == nil {
								report.RepliesRecorded++
								_ = p.outreachStore.UpdateStatus(ctx, parent.ID, model.OutreachStatusReplied, "", "", "", nil)
								// Cancel any pending follow-ups for this contact
								_ = p.outreachStore.CancelPendingFollowups(ctx, parent.ContactID)
								p.logger.Info("reply recorded and pending follow-ups cancelled",
									"outreach_id", parent.ID,
									"contact_id", parent.ContactID,
									"snippet", msg.Snippet,
								)
							}
						}
					}
				}
			}
		}
	}

	// 2. Evaluate Follow-Up 1 Drafts
	if p.outreachStore != nil {
		f1Hours := settings.FollowupAfterHours
		if f1Hours <= 0 {
			f1Hours = 24
		}

		needingF1, err := p.outreachStore.GetSentOutreachNeedingFollowup1(ctx, f1Hours)
		if err == nil {
			for _, o := range needingF1 {
				// Max 2 follow-ups per contact ever
				count, _ := p.outreachStore.CountFollowupsForContact(ctx, o.ContactID)
				if count >= 2 {
					continue
				}

				f1Status := model.OutreachStatusDraft
				if !settings.ApprovalMode {
					f1Status = model.OutreachStatusQueued
				}

				subject := o.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}

				role := o.Role
				if role == "" {
					role = "the role"
				}
				company := o.Company
				if company == "" {
					company = "your team"
				}

				body := fmt.Sprintf(
					"Hi,\n\nFollowing up on my previous email regarding the %s position at %s. I wanted to see if you had a brief moment to review my background and attached resume.\n\nI remain very interested and look forward to hearing from you.\n\nBest regards,",
					role, company,
				)

				_, createErr := p.outreachStore.CreateOutreach(ctx, &model.Outreach{
					ContactID:      o.ContactID,
					PostID:         o.PostID,
					Kind:           model.OutreachKindFollowup1,
					Subject:        subject,
					Body:           body,
					Status:         f1Status,
					GmailThreadID:  o.GmailThreadID,
					GmailMessageID: o.GmailMessageID, // Serves as In-Reply-To reference
				})

				if createErr == nil {
					report.Followup1Drafted++
					p.logger.Info("followup1 draft created", "contact_id", o.ContactID, "status", f1Status)
				}
			}
		}
	}

	// 3. Evaluate Follow-Up 2 Drafts (Final)
	if p.outreachStore != nil {
		needingF2, err := p.outreachStore.GetSentOutreachNeedingFollowup2(ctx, 24)
		if err == nil {
			for _, o := range needingF2 {
				count, _ := p.outreachStore.CountFollowupsForContact(ctx, o.ContactID)
				if count >= 2 {
					continue
				}

				f2Status := model.OutreachStatusDraft
				if !settings.ApprovalMode {
					f2Status = model.OutreachStatusQueued
				}

				subject := o.Subject
				if !strings.HasPrefix(strings.ToLower(subject), "re:") {
					subject = "Re: " + subject
				}

				role := o.Role
				if role == "" {
					role = "the role"
				}
				company := o.Company
				if company == "" {
					company = "your team"
				}

				body := fmt.Sprintf(
					"Hi,\n\nOne final check-in regarding the %s role at %s. If the position has already been filled or priorities have shifted, no worries at all.\n\nThanks again for your time and consideration.\n\nBest regards,",
					role, company,
				)

				_, createErr := p.outreachStore.CreateOutreach(ctx, &model.Outreach{
					ContactID:      o.ContactID,
					PostID:         o.PostID,
					Kind:           model.OutreachKindFollowup2,
					Subject:        subject,
					Body:           body,
					Status:         f2Status,
					GmailThreadID:  o.GmailThreadID,
					GmailMessageID: o.GmailMessageID,
				})

				if createErr == nil {
					report.Followup2Drafted++
					p.logger.Info("followup2 (final) draft created", "contact_id", o.ContactID, "status", f2Status)
				}
			}
		}
	}

	return report, nil
}
