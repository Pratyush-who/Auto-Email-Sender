package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"emailauto-server/internal/gmail"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const OAuthSettingsKey = "google_oauth"

// TokenRepository stores encrypted OAuth credentials in the PostgreSQL settings table.
type TokenRepository struct {
	pool *pgxpool.Pool
}

// NewTokenRepository creates a new TokenRepository.
func NewTokenRepository(pool *pgxpool.Pool) *TokenRepository {
	return &TokenRepository{pool: pool}
}

// SaveToken persists the OAuthTokenData as JSONB in the settings table.
func (r *TokenRepository) SaveToken(ctx context.Context, data *gmail.OAuthTokenData) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal oauth token data: %w", err)
	}

	query := `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`

	_, err = r.pool.Exec(ctx, query, OAuthSettingsKey, raw)
	if err != nil {
		return fmt.Errorf("failed to execute token save query: %w", err)
	}

	return nil
}

// GetToken retrieves the stored OAuthTokenData.
func (r *TokenRepository) GetToken(ctx context.Context) (*gmail.OAuthTokenData, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `SELECT value, updated_at FROM settings WHERE key = $1`

	var raw []byte
	var updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, OAuthSettingsKey).Scan(&raw, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query oauth token: %w", err)
	}

	var data gmail.OAuthTokenData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal oauth token: %w", err)
	}

	data.UpdatedAt = updatedAt
	return &data, nil
}

// DeleteToken removes stored OAuth credentials from the settings table.
func (r *TokenRepository) DeleteToken(ctx context.Context) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `DELETE FROM settings WHERE key = $1`
	_, err := r.pool.Exec(ctx, query, OAuthSettingsKey)
	if err != nil {
		return fmt.Errorf("failed to delete oauth token: %w", err)
	}

	return nil
}
