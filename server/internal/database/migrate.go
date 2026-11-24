package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"emailauto-server/migrations"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// RunMigrations executes database migrations using golang-migrate and embedded SQL files.
func RunMigrations(databaseURL string, logger *slog.Logger) error {
	sourceDriver, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source driver: %w", err)
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to open sql db for migrations: %w", err)
	}
	defer db.Close()

	targetDriver, err := postgres.WithInstance(db, &postgres.Config{
		MigrationsTable: "schema_migrations",
	})
	if err != nil {
		return fmt.Errorf("failed to create postgres migration driver: %w", err)
	}

	m, err := migrate.NewWithInstance(
		"iofs",
		sourceDriver,
		"postgres",
		targetDriver,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize migration instance: %w", err)
	}

	logger.Info("running database migrations...")
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("database migrations already up to date")
			return nil
		}
		return fmt.Errorf("migration up failed: %w", err)
	}

	logger.Info("database migrations applied successfully")
	return nil
}
