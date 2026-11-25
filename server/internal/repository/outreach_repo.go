package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutreachRepository handles Postgres persistence for outreach records.
type OutreachRepository struct {
	pool *pgxpool.Pool
}

// NewOutreachRepository creates an OutreachRepository.
func NewOutreachRepository(pool *pgxpool.Pool) *OutreachRepository {
	return &OutreachRepository{pool: pool}
}

// CreateOutreach inserts a new outreach record into the database.
func (r *OutreachRepository) CreateOutreach(ctx context.Context, o *model.Outreach) (*model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		INSERT INTO outreach (
			contact_id, post_id, kind, subject, body, status,
			gmail_message_id, gmail_thread_id, scheduled_at, sent_at, error,
			created_at, updated_at
		)
		VALUES (
			$1::uuid, NULLIF($2, '')::uuid, $3, $4, $5, $6,
			NULLIF($7, ''), NULLIF($8, ''), $9, $10, NULLIF($11, ''),
			NOW(), NOW()
		)
		RETURNING id, created_at, updated_at
	`

	err := r.pool.QueryRow(
		ctx, query,
		o.ContactID, o.PostID, o.Kind, o.Subject, o.Body, o.Status,
		o.GmailMessageID, o.GmailThreadID, o.ScheduledAt, o.SentAt, o.Error,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create outreach: %w", err)
	}

	return o, nil
}

// GetOutreachByID fetches a single outreach record with joined contact details.
func (r *OutreachRepository) GetOutreachByID(ctx context.Context, id string) (*model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		WHERE o.id = $1::uuid
	`

	var o model.Outreach
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
		&o.GmailMessageID, &o.GmailThreadID,
		&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
		&o.ContactEmail, &o.Company, &o.Role,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get outreach: %w", err)
	}

	return &o, nil
}

// UpdateOutreach updates subject, body, and status of an existing outreach record.
func (r *OutreachRepository) UpdateOutreach(ctx context.Context, o *model.Outreach) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		UPDATE outreach
		SET subject = $2, body = $3, status = $4, updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, o.ID, o.Subject, o.Body, o.Status)
	return err
}

// ListOutreach lists outreach items matching the filter with joined contact and post info.
func (r *OutreachRepository) ListOutreach(ctx context.Context, filter model.OutreachFilter) ([]model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	var conditions []string
	var args []any
	argIdx := 1

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("o.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Kind != "" {
		conditions = append(conditions, fmt.Sprintf("o.kind = $%d", argIdx))
		args = append(args, filter.Kind)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		%s
		ORDER BY o.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list outreach: %w", err)
	}
	defer rows.Close()

	var list []model.Outreach
	for rows.Next() {
		var o model.Outreach
		if err := rows.Scan(
			&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
			&o.GmailMessageID, &o.GmailThreadID,
			&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
			&o.ContactEmail, &o.Company, &o.Role,
		); err != nil {
			return nil, fmt.Errorf("failed to scan outreach row: %w", err)
		}
		list = append(list, o)
	}

	return list, nil
}

// CountSentToday counts how many emails were sent today.
func (r *OutreachRepository) CountSentToday(ctx context.Context) (int, error) {
	if r.pool == nil {
		return 0, errors.New("database pool is not initialized")
	}

	query := `
		SELECT COUNT(*)
		FROM outreach
		WHERE status = 'sent' AND sent_at >= CURRENT_DATE
	`
	var count int
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

// GetSentBodiesToday returns bodies of all drafts or sent emails created today for similarity checks.
func (r *OutreachRepository) GetSentBodiesToday(ctx context.Context) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT body
		FROM outreach
		WHERE created_at >= CURRENT_DATE
		ORDER BY created_at DESC
		LIMIT 50
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bodies []string
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err == nil {
			bodies = append(bodies, b)
		}
	}
	return bodies, nil
}

// HasSentToAddressOrCompanyWithinDays checks if the email or company was emailed within N days.
func (r *OutreachRepository) HasSentToAddressOrCompanyWithinDays(ctx context.Context, email, company string, days int) (bool, error) {
	if r.pool == nil {
		return false, errors.New("database pool is not initialized")
	}

	query := `
		SELECT EXISTS (
			SELECT 1 FROM outreach o
			JOIN contacts c ON o.contact_id = c.id
			WHERE (LOWER(c.email) = LOWER($1) OR (c.company != '' AND LOWER(c.company) = LOWER($2)))
			  AND o.status = 'sent'
			  AND o.sent_at >= NOW() - ($3 || ' days')::INTERVAL
		)
	`
	var exists bool
	err := r.pool.QueryRow(ctx, query, strings.TrimSpace(email), strings.TrimSpace(company), days).Scan(&exists)
	return exists, err
}

// HasExistingInitialOutreach checks if an initial outreach already exists or was sent for this contact.
// If excludeID is provided, that specific outreach record is ignored so an outreach being sent doesn't match itself.
func (r *OutreachRepository) HasExistingInitialOutreach(ctx context.Context, contactID string, excludeID ...string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("database pool is not initialized")
	}

	query := `
		SELECT EXISTS (
			SELECT 1 FROM outreach
			WHERE contact_id = $1::uuid
			  AND kind = 'initial'
			  AND status IN ('queued', 'sent', 'replied')
			  AND ($2::text IS NULL OR $2 = '' OR id != $2::uuid)
		)
	`
	var excl string
	if len(excludeID) > 0 {
		excl = excludeID[0]
	}

	var exists bool
	err := r.pool.QueryRow(ctx, query, contactID, excl).Scan(&exists)
	return exists, err
}

// GetPendingOutreachToSend fetches the next batch of approved or queued outreach ready to be sent.
func (r *OutreachRepository) GetPendingOutreachToSend(ctx context.Context, limit int) ([]model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		WHERE o.status IN ('approved', 'queued')
		ORDER BY o.created_at ASC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pending outreach: %w", err)
	}
	defer rows.Close()

	var list []model.Outreach
	for rows.Next() {
		var o model.Outreach
		if err := rows.Scan(
			&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
			&o.GmailMessageID, &o.GmailThreadID,
			&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
			&o.ContactEmail, &o.Company, &o.Role,
		); err != nil {
			return nil, err
		}
		list = append(list, o)
	}

	return list, nil
}

// UpdateStatus updates the delivery status, timestamps, and identifiers for an outreach.
func (r *OutreachRepository) UpdateStatus(
	ctx context.Context,
	id string,
	status model.OutreachStatus,
	gmailMsgID, gmailThreadID, errText string,
	sentAt *time.Time,
) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		UPDATE outreach
		SET status = $2,
		    gmail_message_id = COALESCE(NULLIF($3, ''), gmail_message_id),
		    gmail_thread_id = COALESCE(NULLIF($4, ''), gmail_thread_id),
		    error = NULLIF($5, ''),
		    sent_at = COALESCE($6, sent_at),
		    updated_at = NOW()
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, id, status, gmailMsgID, gmailThreadID, errText, sentAt)
	return err
}

// MarkContactInvalid marks a contact invalid (e.g. after a hard bounce) by zeroing trust score.
func (r *OutreachRepository) MarkContactInvalid(ctx context.Context, contactID string, reason string) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		UPDATE contacts
		SET trust_score = 0,
		    trust_reasons = trust_reasons || jsonb_build_array($2::text)
		WHERE id = $1::uuid
	`
	_, err := r.pool.Exec(ctx, query, contactID, fmt.Sprintf("invalid/bounced: %s", reason))
	return err
}

// GetSentThreadIDs returns active Gmail thread IDs for sent outreach within N days.
func (r *OutreachRepository) GetSentThreadIDs(ctx context.Context, days int) ([]string, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if days <= 0 {
		days = 30
	}

	query := `
		SELECT DISTINCT gmail_thread_id
		FROM outreach
		WHERE gmail_thread_id IS NOT NULL
		  AND gmail_thread_id != ''
		  AND status IN ('sent', 'replied')
		  AND sent_at >= NOW() - ($1 || ' days')::INTERVAL
	`

	rows, err := r.pool.Query(ctx, query, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threadIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err == nil && tid != "" {
			threadIDs = append(threadIDs, tid)
		}
	}
	return threadIDs, nil
}

// GetOutreachByThreadID finds the parent outreach record associated with a Gmail thread.
func (r *OutreachRepository) GetOutreachByThreadID(ctx context.Context, threadID string) (*model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		WHERE o.gmail_thread_id = $1
		ORDER BY o.created_at ASC
		LIMIT 1
	`

	var o model.Outreach
	err := r.pool.QueryRow(ctx, query, threadID).Scan(
		&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
		&o.GmailMessageID, &o.GmailThreadID,
		&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
		&o.ContactEmail, &o.Company, &o.Role,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

// CancelPendingFollowups cancels any un-sent followups for a contact once a reply is received.
func (r *OutreachRepository) CancelPendingFollowups(ctx context.Context, contactID string) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		UPDATE outreach
		SET status = 'skipped',
		    error = 'reply received, pending follow-up cancelled',
		    updated_at = NOW()
		WHERE contact_id = $1::uuid
		  AND kind IN ('followup1', 'followup2')
		  AND status IN ('draft', 'queued', 'approved')
	`
	_, err := r.pool.Exec(ctx, query, contactID)
	return err
}

// GetSentOutreachNeedingFollowup1 returns sent initial outreaches that have received no reply after N hours.
func (r *OutreachRepository) GetSentOutreachNeedingFollowup1(ctx context.Context, afterHours int) ([]model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if afterHours <= 0 {
		afterHours = 24
	}

	query := `
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		WHERE o.kind = 'initial'
		  AND o.status = 'sent'
		  AND o.sent_at <= NOW() - ($1 || ' hours')::INTERVAL
		  AND NOT EXISTS (
		      SELECT 1 FROM outreach f
		      WHERE f.contact_id = o.contact_id AND f.kind = 'followup1'
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM replies r WHERE r.outreach_id = o.id
		  )
		ORDER BY o.sent_at ASC
		LIMIT 50
	`

	rows, err := r.pool.Query(ctx, query, afterHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Outreach
	for rows.Next() {
		var o model.Outreach
		if err := rows.Scan(
			&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
			&o.GmailMessageID, &o.GmailThreadID,
			&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
			&o.ContactEmail, &o.Company, &o.Role,
		); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, nil
}

// GetSentOutreachNeedingFollowup2 returns sent followup1 outreaches that have received no reply after N hours.
func (r *OutreachRepository) GetSentOutreachNeedingFollowup2(ctx context.Context, afterHours int) ([]model.Outreach, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if afterHours <= 0 {
		afterHours = 24
	}

	query := `
		SELECT o.id, o.contact_id::text, COALESCE(o.post_id::text, ''), o.kind, o.subject, o.body, o.status,
		       COALESCE(o.gmail_message_id, ''), COALESCE(o.gmail_thread_id, ''),
		       o.scheduled_at, o.sent_at, COALESCE(o.error, ''), o.created_at, o.updated_at,
		       c.email, c.company, COALESCE(p.role, '')
		FROM outreach o
		JOIN contacts c ON o.contact_id = c.id
		LEFT JOIN posts p ON o.post_id = p.id
		WHERE o.kind = 'followup1'
		  AND o.status = 'sent'
		  AND o.sent_at <= NOW() - ($1 || ' hours')::INTERVAL
		  AND NOT EXISTS (
		      SELECT 1 FROM outreach f
		      WHERE f.contact_id = o.contact_id AND f.kind = 'followup2'
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM replies r
		      JOIN outreach prev ON r.outreach_id = prev.id
		      WHERE prev.contact_id = o.contact_id
		  )
		ORDER BY o.sent_at ASC
		LIMIT 50
	`

	rows, err := r.pool.Query(ctx, query, afterHours)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []model.Outreach
	for rows.Next() {
		var o model.Outreach
		if err := rows.Scan(
			&o.ID, &o.ContactID, &o.PostID, &o.Kind, &o.Subject, &o.Body, &o.Status,
			&o.GmailMessageID, &o.GmailThreadID,
			&o.ScheduledAt, &o.SentAt, &o.Error, &o.CreatedAt, &o.UpdatedAt,
			&o.ContactEmail, &o.Company, &o.Role,
		); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, nil
}

// CountFollowupsForContact counts existing followups for a contact (max 2 ever).
func (r *OutreachRepository) CountFollowupsForContact(ctx context.Context, contactID string) (int, error) {
	if r.pool == nil {
		return 0, errors.New("database pool is not initialized")
	}

	query := `
		SELECT COUNT(*)
		FROM outreach
		WHERE contact_id = $1::uuid
		  AND kind IN ('followup1', 'followup2')
	`
	var count int
	err := r.pool.QueryRow(ctx, query, contactID).Scan(&count)
	return count, err
}
