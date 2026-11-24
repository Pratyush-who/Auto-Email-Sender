package repository

import (
	"context"
	"errors"
	"fmt"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ReplyRepository manages database access for the replies table.
type ReplyRepository struct {
	pool *pgxpool.Pool
}

// NewReplyRepository creates a new ReplyRepository.
func NewReplyRepository(pool *pgxpool.Pool) *ReplyRepository {
	return &ReplyRepository{pool: pool}
}

// SaveReply inserts a newly detected inbound email reply.
func (r *ReplyRepository) SaveReply(ctx context.Context, reply *model.Reply) (*model.Reply, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		INSERT INTO replies (outreach_id, gmail_message_id, snippet, received_at, created_at)
		VALUES ($1::uuid, $2, $3, $4, NOW())
		RETURNING id, created_at
	`

	err := r.pool.QueryRow(
		ctx, query,
		reply.OutreachID, reply.GmailMessageID, reply.Snippet, reply.ReceivedAt,
	).Scan(&reply.ID, &reply.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to save reply: %w", err)
	}

	return reply, nil
}

// ListReplies returns recent replies joined with contact and outreach context.
func (r *ReplyRepository) ListReplies(ctx context.Context, limit, offset int) ([]model.Reply, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT r.id, r.outreach_id::text, r.gmail_message_id, r.snippet, r.received_at, r.created_at,
		       c.email, c.company, o.subject
		FROM replies r
		JOIN outreach o ON r.outreach_id = o.id
		JOIN contacts c ON o.contact_id = c.id
		ORDER BY r.received_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list replies: %w", err)
	}
	defer rows.Close()

	var list []model.Reply
	for rows.Next() {
		var item model.Reply
		if err := rows.Scan(
			&item.ID, &item.OutreachID, &item.GmailMessageID, &item.Snippet, &item.ReceivedAt, &item.CreatedAt,
			&item.ContactEmail, &item.Company, &item.Subject,
		); err != nil {
			return nil, fmt.Errorf("failed to scan reply row: %w", err)
		}
		list = append(list, item)
	}

	return list, nil
}

// HasReplyForOutreach checks if an outreach has already received an inbound reply.
func (r *ReplyRepository) HasReplyForOutreach(ctx context.Context, outreachID string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("database pool is not initialized")
	}

	query := `SELECT EXISTS (SELECT 1 FROM replies WHERE outreach_id = $1::uuid)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, outreachID).Scan(&exists)
	return exists, err
}
