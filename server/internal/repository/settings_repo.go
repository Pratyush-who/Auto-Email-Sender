package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SettingsKey = "app_settings"

// SettingsRepository handles storage and retrieval of application Settings in PostgreSQL.
type SettingsRepository struct {
	pool *pgxpool.Pool
}

// NewSettingsRepository creates a new SettingsRepository.
func NewSettingsRepository(pool *pgxpool.Pool) *SettingsRepository {
	return &SettingsRepository{pool: pool}
}

// Get loads settings from database or returns DefaultSettings if none found.
func (r *SettingsRepository) Get(ctx context.Context) (*model.Settings, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `SELECT value, updated_at FROM settings WHERE key = $1`

	var raw []byte
	var updatedAt time.Time

	err := r.pool.QueryRow(ctx, query, SettingsKey).Scan(&raw, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			defaults := model.DefaultSettings()
			return &defaults, nil
		}
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	defaults := model.DefaultSettings()
	if err := json.Unmarshal(raw, &defaults); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings json: %w", err)
	}

	defaults.UpdatedAt = updatedAt
	return &defaults, nil
}

// Save stores updated settings as JSONB in PostgreSQL.
func (r *SettingsRepository) Save(ctx context.Context, settings *model.Settings) (*model.Settings, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING updated_at
	`

	var updatedAt time.Time
	err = r.pool.QueryRow(ctx, query, SettingsKey, raw).Scan(&updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save settings: %w", err)
	}

	settings.UpdatedAt = updatedAt
	return settings, nil
}

// GetSettingRaw retrieves the raw JSON bytes for a specific setting key.
func (r *SettingsRepository) GetSettingRaw(ctx context.Context, key string) ([]byte, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `SELECT value FROM settings WHERE key = $1`
	var raw []byte
	err := r.pool.QueryRow(ctx, query, key).Scan(&raw)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// SaveSettingRaw stores raw JSON bytes under a specific setting key.
func (r *SettingsRepository) SaveSettingRaw(ctx context.Context, key string, value []byte) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	query := `
		INSERT INTO settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key)
		DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`
	_, err := r.pool.Exec(ctx, query, key, value)
	return err
}
