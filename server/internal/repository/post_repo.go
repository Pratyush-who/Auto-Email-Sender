package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"emailauto-server/internal/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostRepository handles database queries for posts and runs.
type PostRepository struct {
	pool *pgxpool.Pool
}

// NewPostRepository creates a new PostRepository.
func NewPostRepository(pool *pgxpool.Pool) *PostRepository {
	return &PostRepository{pool: pool}
}

// SavePost inserts or updates a post identified by unique URL.
func (r *PostRepository) SavePost(ctx context.Context, p *model.Post) (*model.Post, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	extractedJSON, err := json.Marshal(p.Extracted)
	if err != nil {
		extractedJSON = []byte("{}")
	}

	query := `
		INSERT INTO posts (
			url, author, text, company, role, work_mode, paid_status, location, extracted, status, skip_reason, found_at, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, COALESCE($12, NOW()), NOW()
		)
		ON CONFLICT (url) DO UPDATE SET
			text = EXCLUDED.text,
			author = COALESCE(NULLIF(EXCLUDED.author, ''), posts.author),
			company = COALESCE(NULLIF(EXCLUDED.company, ''), posts.company),
			role = COALESCE(NULLIF(EXCLUDED.role, ''), posts.role),
			work_mode = COALESCE(NULLIF(EXCLUDED.work_mode, ''), posts.work_mode),
			paid_status = COALESCE(NULLIF(EXCLUDED.paid_status, ''), posts.paid_status),
			location = COALESCE(NULLIF(EXCLUDED.location, ''), posts.location),
			extracted = EXCLUDED.extracted,
			status = EXCLUDED.status,
			skip_reason = EXCLUDED.skip_reason
		RETURNING id, url, author, text, company, role, work_mode, paid_status, location, extracted, status, skip_reason, found_at, created_at
	`

	var saved model.Post
	var rawExtracted []byte
	var skipReason *string

	err = r.pool.QueryRow(
		ctx, query,
		p.URL, p.Author, p.Text, p.Company, p.Role, p.WorkMode, p.PaidStatus, p.Location,
		extractedJSON, p.Status, p.SkipReason, p.FoundAt,
	).Scan(
		&saved.ID, &saved.URL, &saved.Author, &saved.Text, &saved.Company, &saved.Role,
		&saved.WorkMode, &saved.PaidStatus, &saved.Location, &rawExtracted, &saved.Status,
		&skipReason, &saved.FoundAt, &saved.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to save post: %w", err)
	}

	if skipReason != nil {
		saved.SkipReason = *skipReason
	}
	_ = json.Unmarshal(rawExtracted, &saved.Extracted)

	return &saved, nil
}

// GetPosts returns a list of posts ordered by found_at descending.
func (r *PostRepository) GetPosts(ctx context.Context, limit, offset int) ([]model.Post, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, url, author, text, company, role, work_mode, paid_status, location, extracted, status, skip_reason, found_at, created_at
		FROM posts
		ORDER BY found_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query posts: %w", err)
	}
	defer rows.Close()

	var posts []model.Post
	for rows.Next() {
		var p model.Post
		var rawExtracted []byte
		var skipReason *string

		if err := rows.Scan(
			&p.ID, &p.URL, &p.Author, &p.Text, &p.Company, &p.Role,
			&p.WorkMode, &p.PaidStatus, &p.Location, &rawExtracted, &p.Status,
			&skipReason, &p.FoundAt, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan post row: %w", err)
		}

		if skipReason != nil {
			p.SkipReason = *skipReason
		}
		_ = json.Unmarshal(rawExtracted, &p.Extracted)
		posts = append(posts, p)
	}

	return posts, nil
}

// HasPostWithURL checks whether a post URL has already been scraped and stored.
func (r *PostRepository) HasPostWithURL(ctx context.Context, postURL string) (bool, error) {
	if r.pool == nil {
		return false, errors.New("database pool is not initialized")
	}

	query := `SELECT EXISTS (SELECT 1 FROM posts WHERE url = $1)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, strings.TrimSpace(postURL)).Scan(&exists)
	return exists, err
}

// CreateRun inserts a new run into the runs table.
func (r *PostRepository) CreateRun(ctx context.Context, run *model.Run) (*model.Run, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	statsJSON, err := json.Marshal(run.Stats)
	if err != nil {
		statsJSON = []byte("{}")
	}

	query := `
		INSERT INTO runs (query, time_filter, sort_by, trigger, status, started_at, stats, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id, query, time_filter, sort_by, trigger, status, started_at, finished_at, stats, created_at
	`

	var created model.Run
	var rawStats []byte

	err = r.pool.QueryRow(
		ctx, query,
		run.Query, run.TimeFilter, run.SortBy, run.Trigger, run.Status, run.StartedAt, statsJSON,
	).Scan(
		&created.ID, &created.Query, &created.TimeFilter, &created.SortBy, &created.Trigger,
		&created.Status, &created.StartedAt, &created.FinishedAt, &rawStats, &created.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to insert run: %w", err)
	}

	_ = json.Unmarshal(rawStats, &created.Stats)
	return &created, nil
}

// UpdateRun updates status, finished_at, and stats of an existing run.
func (r *PostRepository) UpdateRun(ctx context.Context, runID, status string, finishedAt time.Time, stats map[string]any) error {
	if r.pool == nil {
		return errors.New("database pool is not initialized")
	}

	statsJSON, err := json.Marshal(stats)
	if err != nil {
		statsJSON = []byte("{}")
	}

	query := `
		UPDATE runs
		SET status = $1, finished_at = $2, stats = $3
		WHERE id = $4
	`

	_, err = r.pool.Exec(ctx, query, status, finishedAt, statsJSON, runID)
	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	return nil
}

// GetRuns returns recent runs ordered by created_at descending.
func (r *PostRepository) GetRuns(ctx context.Context, limit int) ([]model.Run, error) {
	if r.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}

	if limit <= 0 {
		limit = 20
	}

	query := `
		SELECT id, query, time_filter, sort_by, trigger, status, started_at, finished_at, stats, created_at
		FROM runs
		ORDER BY created_at DESC
		LIMIT $1
	`

	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query runs: %w", err)
	}
	defer rows.Close()

	var runs []model.Run
	for rows.Next() {
		var run model.Run
		var rawStats []byte

		if err := rows.Scan(
			&run.ID, &run.Query, &run.TimeFilter, &run.SortBy, &run.Trigger,
			&run.Status, &run.StartedAt, &run.FinishedAt, &rawStats, &run.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan run row: %w", err)
		}

		_ = json.Unmarshal(rawStats, &run.Stats)
		runs = append(runs, run)
	}

	return runs, nil
}
