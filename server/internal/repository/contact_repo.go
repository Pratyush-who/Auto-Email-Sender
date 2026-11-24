package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ContactRepository handles database queries for contacts.
type ContactRepository struct {
	pool *pgxpool.Pool
}

// NewContactRepository creates a new ContactRepository.
func NewContactRepository(pool *pgxpool.Pool) *ContactRepository {
	return &ContactRepository{pool: pool}
}

// SaveContact inserts or updates a contact identified by unique email.
func (r *ContactRepository) SaveContact(ctx context.Context, c *model.Contact) (*model.Contact, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	reasonsJSON, err := json.Marshal(c.TrustReasons)
	if err != nil {
		reasonsJSON = []byte("[]")
	}

	query := `
		INSERT INTO contacts (email, company, domain, trust_score, trust_reasons, post_id, created_at)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, NOW())
		ON CONFLICT (email) DO UPDATE SET
			company = COALESCE(NULLIF(EXCLUDED.company, ''), contacts.company),
			domain = COALESCE(NULLIF(EXCLUDED.domain, ''), contacts.domain),
			trust_score = EXCLUDED.trust_score,
			trust_reasons = EXCLUDED.trust_reasons,
			post_id = COALESCE(EXCLUDED.post_id, contacts.post_id)
		RETURNING id, email, company, domain, trust_score, trust_reasons, post_id::text, created_at
	`

	var saved model.Contact
	var rawReasons []byte
	var postID *string

	err = r.pool.QueryRow(
		ctx, query,
		c.Email, c.Company, c.Domain, c.TrustScore, reasonsJSON, c.PostID,
	).Scan(
		&saved.ID, &saved.Email, &saved.Company, &saved.Domain,
		&saved.TrustScore, &rawReasons, &postID, &saved.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to save contact: %w", err)
	}

	if postID != nil {
		saved.PostID = *postID
	}
	_ = json.Unmarshal(rawReasons, &saved.TrustReasons)

	return &saved, nil
}

// GetContacts retrieves contacts ordered by creation time descending.
func (r *ContactRepository) GetContacts(ctx context.Context, limit int) ([]model.Contact, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, email, company, domain, trust_score, trust_reasons, post_id::text, created_at
		FROM contacts
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", err)
	}
	defer rows.Close()

	var contacts []model.Contact
	for rows.Next() {
		var c model.Contact
		var rawReasons []byte
		var postID *string

		if err := rows.Scan(&c.ID, &c.Email, &c.Company, &c.Domain, &c.TrustScore, &rawReasons, &postID, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan contact: %w", err)
		}
		if postID != nil {
			c.PostID = *postID
		}
		_ = json.Unmarshal(rawReasons, &c.TrustReasons)
		contacts = append(contacts, c)
	}

	return contacts, nil
}
