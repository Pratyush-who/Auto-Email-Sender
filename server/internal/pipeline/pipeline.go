package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"emailauto-server/internal/composer"
	"emailauto-server/internal/filter"
	"emailauto-server/internal/model"
)

// PostStore defines repository methods needed by the pipeline.
type PostStore interface {
	SavePost(ctx context.Context, p *model.Post) (*model.Post, error)
}

// ContactStore defines contact persistence needed by the pipeline.
type ContactStore interface {
	SaveContact(ctx context.Context, c *model.Contact) (*model.Contact, error)
}

// ProfileStore defines candidate profile access.
type ProfileStore interface {
	Get(ctx context.Context) (*model.Profile, error)
}

// OutreachStore defines outreach persistence.
type OutreachStore interface {
	CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error)
	GetSentBodiesToday(ctx context.Context) ([]string, error)
	HasExistingInitialOutreach(ctx context.Context, contactID string) (bool, error)
}

// ComposerService defines email drafting capabilities.
type ComposerService interface {
	Compose(ctx context.Context, profile model.Profile, post model.Post, companySummary string, existingBodiesToday []string) (*composer.ComposedEmail, error)
}

// ProcessPostResult captures the outcome of processing a post.
type ProcessPostResult struct {
	Post     *model.Post
	Contact  *model.Contact
	Outreach *model.Outreach
	Passed   bool
	Reason   string
}

// Pipeline orchestrates extraction, filtering, verification, and database persistence.
type Pipeline struct {
	extractor     *PostExtractor
	verifier      *CompanyVerifier
	postStore     PostStore
	contactStore  ContactStore
	profileStore  ProfileStore
	outreachStore OutreachStore
	composer      ComposerService
	logger        *slog.Logger
}

// NewPipeline creates a new extraction and verification Pipeline.
func NewPipeline(
	extractor *PostExtractor,
	verifier *CompanyVerifier,
	postStore PostStore,
	contactStore ContactStore,
	logger *slog.Logger,
) *Pipeline {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pipeline{
		extractor:    extractor,
		verifier:     verifier,
		postStore:    postStore,
		contactStore: contactStore,
		logger:       logger,
	}
}

// WithComposer hooks automated drafting capabilities into the pipeline.
func (p *Pipeline) WithComposer(c ComposerService, ps ProfileStore, os OutreachStore) *Pipeline {
	p.composer = c
	p.profileStore = ps
	p.outreachStore = os
	return p
}

// ProcessPost runs extraction, filtering, verification, and persistence for a single post.
func (p *Pipeline) ProcessPost(ctx context.Context, post *model.Post, settings model.Settings) (*ProcessPostResult, error) {
	if post == nil {
		return nil, fmt.Errorf("post cannot be nil")
	}

	p.logger.Info("processing post", "url", post.URL, "author", post.Author)

	// Step 1: Extraction via LLM + Regex Cross-Checking
	extracted, err := p.extractor.ExtractPost(ctx, post.Text)
	if err != nil {
		p.logger.Error("extraction failed for post", "url", post.URL, "error", err)
		// Mark post as skipped on fatal extraction error
		post.Status = "skipped"
		post.SkipReason = fmt.Sprintf("extraction error: %v", err)
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: post.SkipReason}, nil
	}

	// Enrich post with extracted metadata
	if strings.TrimSpace(post.Company) == "" {
		post.Company = extracted.Company
	}
	if strings.TrimSpace(post.Role) == "" {
		post.Role = extracted.Role
	}
	if strings.TrimSpace(post.WorkMode) == "" {
		post.WorkMode = extracted.WorkMode
	}
	if strings.TrimSpace(post.Location) == "" {
		post.Location = extracted.Location
	}
	if strings.TrimSpace(post.PaidStatus) == "" {
		post.PaidStatus = extracted.PaidStatus
	}

	post.Extracted.TechStack = extracted.TechStack
	post.Extracted.Emails = extracted.Emails
	post.Extracted.RequiredSubjectLine = extracted.RequiredSubjectLine
	post.Extracted.ApplyInstructions = extracted.ApplyInstructions
	if extracted.StipendAmount != nil {
		post.Extracted.Stipend = extracted.StipendAmount
	}
	if extracted.ExperienceYears != nil {
		post.Extracted.ExperienceYears = extracted.ExperienceYears
	}

	// Step 2: Check if post was flagged as suspicious scam or fraudulent
	if extracted.IsSuspicious {
		post.Extracted.IsSuspicious = true
		post.Extracted.SuspicionReason = extracted.SuspicionReason
		post.Status = "skipped"
		post.SkipReason = fmt.Sprintf("SPAM / FAKE POST FLAGGED: %s", extracted.SuspicionReason)
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: post.SkipReason}, nil
	}

	// Step 3: Check if hiring post
	if !extracted.IsHiringPost {
		post.Status = "skipped"
		post.SkipReason = "post is not a hiring or job announcement"
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: post.SkipReason}, nil
	}

	// Step 4: Check if valid contact emails exist
	if len(extracted.Emails) == 0 {
		post.Status = "skipped"
		post.SkipReason = "no application email found in post"
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: post.SkipReason}, nil
	}

	// Step 5: Apply filter rules against settings (experience, stipend, keywords, work mode, etc.)
	passFilter, filterReason := filter.ApplyFilters(*post, settings)
	if !passFilter {
		post.Status = "skipped"
		post.SkipReason = filterReason
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: filterReason}, nil
	}

	// Step 6: Verification (Company & Domain, DNS/MX, Red flags, 2-line summary)
	primaryEmail := extracted.Emails[0]
	emailParts := strings.Split(primaryEmail, "@")
	domain := ""
	if len(emailParts) == 2 {
		domain = emailParts[1]
	}

	verResult, verErr := p.verifier.Verify(ctx, post.Company, domain, post.Text)
	if verErr != nil {
		p.logger.Warn("verification error", "domain", domain, "error", verErr)
	}

	if verResult != nil {
		post.Extracted.CompanySummary = verResult.CompanySummary
		post.Extracted.RedFlags = verResult.RedFlags

		// Explicit check for red flags detected during due diligence
		if len(verResult.RedFlags) > 0 {
			post.Extracted.IsSuspicious = true
			post.Extracted.SuspicionReason = fmt.Sprintf("Red flags detected: %s", strings.Join(verResult.RedFlags, "; "))
			post.Status = "skipped"
			post.SkipReason = fmt.Sprintf("SPAM / FAKE POST FLAGGED: %s", strings.Join(verResult.RedFlags, "; "))

			if p.postStore != nil {
				post, _ = p.postStore.SavePost(ctx, post)
			}

			var contact *model.Contact
			if p.contactStore != nil {
				contact, _ = p.contactStore.SaveContact(ctx, &model.Contact{
					Email:        primaryEmail,
					Company:      post.Company,
					Domain:       verResult.Domain,
					TrustScore:   float64(verResult.TrustScore),
					TrustReasons: verResult.TrustReasons,
					PostID:       post.ID,
				})
			}

			return &ProcessPostResult{
				Post:    post,
				Contact: contact,
				Passed:  false,
				Reason:  post.SkipReason,
			}, nil
		}

		// Check Trust Score against threshold
		if verResult.TrustScore < settings.TrustThreshold {
			post.Extracted.IsSuspicious = true
			post.Extracted.SuspicionReason = fmt.Sprintf("Trust score %d is below threshold %d", verResult.TrustScore, settings.TrustThreshold)
			post.Status = "skipped"
			post.SkipReason = fmt.Sprintf("trust score %d is below threshold %d (%s)",
				verResult.TrustScore, settings.TrustThreshold, strings.Join(verResult.TrustReasons, "; "))

			if p.postStore != nil {
				post, _ = p.postStore.SavePost(ctx, post)
			}

			// Store contact even for skipped so user can audit trust score
			var contact *model.Contact
			if p.contactStore != nil {
				contact, _ = p.contactStore.SaveContact(ctx, &model.Contact{
					Email:        primaryEmail,
					Company:      post.Company,
					Domain:       verResult.Domain,
					TrustScore:   float64(verResult.TrustScore),
					TrustReasons: verResult.TrustReasons,
					PostID:       post.ID,
				})
			}

			return &ProcessPostResult{
				Post:    post,
				Contact: contact,
				Passed:  false,
				Reason:  post.SkipReason,
			}, nil
		}
	}

	// Step 6: Passed all checks
	post.Status = "passed"
	post.SkipReason = ""

	if p.postStore != nil {
		savedPost, err := p.postStore.SavePost(ctx, post)
		if err == nil && savedPost != nil {
			post = savedPost
		}
	}

	// Save contact with trust score, reasons, and post_id
	var contact *model.Contact
	if p.contactStore != nil && verResult != nil {
		savedContact, err := p.contactStore.SaveContact(ctx, &model.Contact{
			Email:        primaryEmail,
			Company:      post.Company,
			Domain:       verResult.Domain,
			TrustScore:   float64(verResult.TrustScore),
			TrustReasons: verResult.TrustReasons,
			PostID:       post.ID,
		})
		if err == nil {
			contact = savedContact
		}
	}

	// Step 7: Automated Draft Composition for Passed Post
	var outreach *model.Outreach
	if p.composer != nil && p.profileStore != nil && p.outreachStore != nil && contact != nil {
		profile, err := p.profileStore.Get(ctx)
		if err == nil && profile != nil {
			hasInit, _ := p.outreachStore.HasExistingInitialOutreach(ctx, contact.ID)
			if !hasInit {
				bodiesToday, _ := p.outreachStore.GetSentBodiesToday(ctx)
				summary := ""
				if verResult != nil {
					summary = verResult.CompanySummary
				}
				compEmail, compErr := p.composer.Compose(ctx, *profile, *post, summary, bodiesToday)
				if compErr == nil && compEmail != nil {
					status := model.OutreachStatusQueued
					if settings.ApprovalMode {
						status = model.OutreachStatusDraft
					}
					created, err := p.outreachStore.CreateOutreach(ctx, &model.Outreach{
						ContactID: contact.ID,
						PostID:    post.ID,
						Kind:      model.OutreachKindInitial,
						Subject:   compEmail.Subject,
						Body:      compEmail.Body,
						Status:    status,
					})
					if err == nil {
						outreach = created
					}
					p.logger.Info("automated outreach draft generated",
						"outreach_id", outreach.ID,
						"status", status,
						"subject", compEmail.Subject,
					)
				}
			}
		}
	}

	p.logger.Info("post passed pipeline checks",
		"url", post.URL,
		"company", post.Company,
		"email", primaryEmail,
		"trust_score", verResult.TrustScore,
	)

	return &ProcessPostResult{
		Post:     post,
		Contact:  contact,
		Outreach: outreach,
		Passed:   true,
	}, nil
}

// ProcessBatch processes a slice of posts, throttling between calls to stay comfortably under rate limits.
func (p *Pipeline) ProcessBatch(ctx context.Context, posts []model.Post, settings model.Settings) ([]ProcessPostResult, error) {
	results := make([]ProcessPostResult, 0, len(posts))

	for i := range posts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		res, err := p.ProcessPost(ctx, &posts[i], settings)
		if err != nil {
			p.logger.Error("error processing batch post", "index", i, "error", err)
			continue
		}
		if res != nil {
			results = append(results, *res)
		}

		// Small pacing delay between batch posts to prevent bursting against LLM rate limits
		if i < len(posts)-1 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	return results, nil
}
