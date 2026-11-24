package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProfileRepository handles database queries for the Profile entity.
type ProfileRepository struct {
	pool *pgxpool.Pool
}

// NewProfileRepository creates a new ProfileRepository.
func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

// Get returns the single active candidate profile or a default empty profile.
func (r *ProfileRepository) Get(ctx context.Context) (*model.Profile, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	query := `
		SELECT id, summary, skills, resume_path, resume_text, example_text, created_at, updated_at
		FROM profile
		ORDER BY created_at ASC
		LIMIT 1
	`

	var p model.Profile
	err := r.pool.QueryRow(ctx, query).Scan(
		&p.ID,
		&p.Summary,
		&p.Skills,
		&p.ResumePath,
		&p.ResumeText,
		&p.ExampleText,
		&p.CreatedAt,
		&p.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &model.Profile{
				Summary:     "",
				Skills:      "",
				ResumePath:  "",
				ResumeText:  "",
				ExampleText: "",
				CreatedAt:   time.Now().UTC(),
				UpdatedAt:   time.Now().UTC(),
			}, nil
		}
		return nil, fmt.Errorf("failed to query profile: %w", err)
	}

	return &p, nil
}

// Save updates the existing profile or creates one if none exists.
func (r *ProfileRepository) Save(ctx context.Context, input *model.ProfileInput) (*model.Profile, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	existing, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}

	if existing.ID != "" {
		// Update existing
		query := `
			UPDATE profile
			SET summary = $1, skills = $2, example_text = $3, updated_at = NOW()
			WHERE id = $4
			RETURNING id, summary, skills, resume_path, resume_text, example_text, created_at, updated_at
		`
		var updated model.Profile
		err = r.pool.QueryRow(ctx, query, input.Summary, input.Skills, input.ExampleText, existing.ID).Scan(
			&updated.ID,
			&updated.Summary,
			&updated.Skills,
			&updated.ResumePath,
			&updated.ResumeText,
			&updated.ExampleText,
			&updated.CreatedAt,
			&updated.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update profile: %w", err)
		}
		return &updated, nil
	}

	// Insert new
	query := `
		INSERT INTO profile (summary, skills, example_text, resume_path, resume_text)
		VALUES ($1, $2, $3, '', '')
		RETURNING id, summary, skills, resume_path, resume_text, example_text, created_at, updated_at
	`
	var inserted model.Profile
	err = r.pool.QueryRow(ctx, query, input.Summary, input.Skills, input.ExampleText).Scan(
		&inserted.ID,
		&inserted.Summary,
		&inserted.Skills,
		&inserted.ResumePath,
		&inserted.ResumeText,
		&inserted.ExampleText,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert profile: %w", err)
	}
	return &inserted, nil
}

// UpdateResume updates resume_path and resume_text for the profile.
func (r *ProfileRepository) UpdateResume(ctx context.Context, resumePath, resumeText string) (*model.Profile, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	existing, err := r.Get(ctx)
	if err != nil {
		return nil, err
	}

	if existing.ID != "" {
		query := `
			UPDATE profile
			SET resume_path = $1, resume_text = $2, updated_at = NOW()
			WHERE id = $3
			RETURNING id, summary, skills, resume_path, resume_text, example_text, created_at, updated_at
		`
		var updated model.Profile
		err = r.pool.QueryRow(ctx, query, resumePath, resumeText, existing.ID).Scan(
			&updated.ID,
			&updated.Summary,
			&updated.Skills,
			&updated.ResumePath,
			&updated.ResumeText,
			&updated.ExampleText,
			&updated.CreatedAt,
			&updated.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update resume: %w", err)
		}
		return &updated, nil
	}

	query := `
		INSERT INTO profile (summary, skills, example_text, resume_path, resume_text)
		VALUES ('', '', '', $1, $2)
		RETURNING id, summary, skills, resume_path, resume_text, example_text, created_at, updated_at
	`
	var inserted model.Profile
	err = r.pool.QueryRow(ctx, query, resumePath, resumeText).Scan(
		&inserted.ID,
		&inserted.Summary,
		&inserted.Skills,
		&inserted.ResumePath,
		&inserted.ResumeText,
		&inserted.ExampleText,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert resume into profile: %w", err)
	}
	return &inserted, nil
}
