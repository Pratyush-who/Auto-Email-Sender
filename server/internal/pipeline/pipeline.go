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
	HasExistingInitialOutreach(ctx context.Context, contactID string, excludeID ...string) (bool, error)
}

// ComposerService defines email drafting capabilities.
type ComposerService interface {
	Compose(ctx context.Context, profile model.Profile, post model.Post, companySummary string, existingBodiesToday []string) (*composer.ComposedEmail, error)
	ComposeBatch(ctx context.Context, profile model.Profile, items []composer.BatchComposeItem, existingBodiesToday []string) (map[string]composer.ComposedEmail, error)
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
	return p.processPostInternal(ctx, post, settings, true)
}

func (p *Pipeline) processPostInternal(ctx context.Context, post *model.Post, settings model.Settings, draftImmediately bool) (*ProcessPostResult, error) {
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
	post.Extracted.Phones = extracted.Phones
	post.Extracted.RequiredSubjectLine = extracted.RequiredSubjectLine
	post.Extracted.ApplyInstructions = extracted.ApplyInstructions
	post.Extracted.ApplyViaDM = extracted.ApplyViaDM
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

	// Step 4: Check if valid contact emails exist (surface Phone or DM instructions clearly if no email)
	if len(extracted.Emails) == 0 {
		post.Status = "skipped"
		if len(extracted.Phones) > 0 {
			post.SkipReason = "no email found — contact via Phone / WhatsApp"
		} else if extracted.ApplyViaDM {
			post.SkipReason = "no email found — DM directly on LinkedIn"
		} else {
			post.SkipReason = "no application email found in post"
		}
		if p.postStore != nil {
			post, _ = p.postStore.SavePost(ctx, post)
		}
		return &ProcessPostResult{Post: post, Passed: false, Reason: post.SkipReason}, nil
	}

	// Step 5: Apply filter rules against settings.
	// Always allow onsite/hybrid/remote and treat unmentioned stipend as paid (skip only if explicitly unpaid).
	effectiveSettings := settings
	effectiveSettings.WorkModes = nil
	effectiveSettings.SkipUnknownPaid = false
	passFilter, filterReason := filter.ApplyFilters(*post, effectiveSettings)
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

		// Explicit check for severe scam/fraud keywords detected during due diligence
		isDefinitiveScam := false
		scamIndicators := []string{
			"registration fee", "pay to apply", "processing fee", "deposit required",
			"crypto payment", "wire transfer", "telegram task", "whatsapp task",
		}
		for _, rf := range verResult.RedFlags {
			rfLower := strings.ToLower(rf)
			for _, scamWord := range scamIndicators {
				if strings.Contains(rfLower, scamWord) {
					isDefinitiveScam = true
					break
				}
			}
		}

		if isDefinitiveScam {
			post.Extracted.IsSuspicious = true
			post.Extracted.SuspicionReason = fmt.Sprintf("Fraudulent activity flagged: %s", strings.Join(verResult.RedFlags, "; "))
			post.Status = "skipped"
			post.SkipReason = fmt.Sprintf("SPAM / SCAM FLAGGED: %s", strings.Join(verResult.RedFlags, "; "))

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

	// Step 7: Automated Draft Composition for Passed Post (when not deferred to batch drafting)
	var outreach *model.Outreach
	if draftImmediately && p.composer != nil && p.profileStore != nil && p.outreachStore != nil && contact != nil {
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
					if err == nil && created != nil {
						outreach = created
						p.logger.Info("automated outreach draft generated",
							"outreach_id", outreach.ID,
							"status", status,
							"subject", compEmail.Subject,
						)
					}
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

// ProcessAndDraftBatch processes all posts in order, deduplicates contacts by email,
// and generates company-specific drafts in batches of 10 per Gemini API call.
func (p *Pipeline) ProcessAndDraftBatch(ctx context.Context, posts []model.Post, settings model.Settings) ([]ProcessPostResult, error) {
	results := make([]ProcessPostResult, 0, len(posts))
	if len(posts) == 0 {
		return results, nil
	}

	type pendingDraft struct {
		resultIdx int
		item      composer.BatchComposeItem
		contact   *model.Contact
	}

	var toDraft []pendingDraft
	seenEmails := make(map[string]bool)

	for i := range posts {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		res, err := p.processPostInternal(ctx, &posts[i], settings, false)
		if err != nil {
			results = append(results, ProcessPostResult{
				Post:   &posts[i],
				Passed: false,
				Reason: err.Error(),
			})
			continue
		}
		if res == nil {
			continue
		}

		idx := len(results)
		results = append(results, *res)

		if res.Passed && res.Contact != nil && res.Post != nil {
			emailKey := strings.ToLower(strings.TrimSpace(res.Contact.Email))
			if emailKey == "" || seenEmails[emailKey] {
				p.logger.Info("skipping duplicate email draft in batch", "email", emailKey, "post_id", res.Post.ID)
				continue
			}

			if p.outreachStore != nil {
				hasInit, _ := p.outreachStore.HasExistingInitialOutreach(ctx, res.Contact.ID)
				if hasInit {
					p.logger.Info("skipping draft: initial outreach already exists for email", "email", emailKey)
					seenEmails[emailKey] = true
					continue
				}
			}

			seenEmails[emailKey] = true
			toDraft = append(toDraft, pendingDraft{
				resultIdx: idx,
				item: composer.BatchComposeItem{
					PostID:         res.Post.ID,
					Post:           *res.Post,
					CompanySummary: res.Post.Extracted.CompanySummary,
				},
				contact: res.Contact,
			})
		}
	}

	// Batch draft with Gemini dynamically based on total posts (up to 25 posts per Gemini API call)
	if len(toDraft) > 0 && p.composer != nil && p.profileStore != nil && p.outreachStore != nil {
		profile, err := p.profileStore.Get(ctx)
		if err == nil && profile != nil {
			bodiesToday, _ := p.outreachStore.GetSentBodiesToday(ctx)
			batchSize := len(toDraft)
			if batchSize > 15 {
				batchSize = 15
			}

			for start := 0; start < len(toDraft); start += batchSize {
				end := start + batchSize
				if end > len(toDraft) {
					end = len(toDraft)
				}
				chunk := toDraft[start:end]

				batchItems := make([]composer.BatchComposeItem, len(chunk))
				for j, pd := range chunk {
					batchItems[j] = pd.item
				}

				composedMap, compErr := p.composer.ComposeBatch(ctx, *profile, batchItems, bodiesToday)
				if compErr != nil {
					p.logger.Warn("batch compose failed", "error", compErr)
					continue
				}

				status := model.OutreachStatusQueued
				if settings.ApprovalMode {
					status = model.OutreachStatusDraft
				}

				for _, pd := range chunk {
					compEmail, ok := composedMap[pd.item.PostID]
					if !ok || strings.TrimSpace(compEmail.Body) == "" {
						continue
					}

					created, err := p.outreachStore.CreateOutreach(ctx, &model.Outreach{
						ContactID: pd.contact.ID,
						PostID:    pd.item.PostID,
						Kind:      model.OutreachKindInitial,
						Subject:   compEmail.Subject,
						Body:      compEmail.Body,
						Status:    status,
					})
					if err == nil && created != nil {
						results[pd.resultIdx].Outreach = created
					}
				}
			}
		}
	}

	return results, nil
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
