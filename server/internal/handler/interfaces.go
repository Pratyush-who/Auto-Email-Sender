package handler

import (
	"context"

	"emailauto-server/internal/model"
)

// ProfileStore defines methods needed by ProfileHandler.
type ProfileStore interface {
	Get(ctx context.Context) (*model.Profile, error)
	Save(ctx context.Context, input *model.ProfileInput) (*model.Profile, error)
	UpdateResume(ctx context.Context, resumePath, resumeText string) (*model.Profile, error)
}

// SettingsStore defines methods needed by SettingsHandler.
type SettingsStore interface {
	Get(ctx context.Context) (*model.Settings, error)
	Save(ctx context.Context, settings *model.Settings) (*model.Settings, error)
}
