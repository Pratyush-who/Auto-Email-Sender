package config

import (
	"os"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	os.Unsetenv("PORT")
	os.Unsetenv("SERVER_PORT")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("CORS_ALLOWED_ORIGINS")
	os.Unsetenv("LOG_LEVEL")
	os.Unsetenv("ENV")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %s", cfg.Port)
	}
	if len(cfg.CORSAllowedOrigins) == 0 {
		t.Errorf("expected default CORS origins to not be empty")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default log level info, got %s", cfg.LogLevel)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected default env development, got %s", cfg.Environment)
	}
	if !cfg.MigrateOnStartup {
		t.Errorf("expected MigrateOnStartup to default to true")
	}
}

func TestConfigCustomEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://custom:custom@localhost:5433/custom?sslmode=disable")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://frontend.local:3000,http://another.local")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ENV", "production")
	t.Setenv("MIGRATE_ON_STARTUP", "false")

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("expected port 9090, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://custom:custom@localhost:5433/custom?sslmode=disable" {
		t.Errorf("expected custom database url, got %s", cfg.DatabaseURL)
	}
	if len(cfg.CORSAllowedOrigins) != 2 || cfg.CORSAllowedOrigins[0] != "http://frontend.local:3000" {
		t.Errorf("unexpected CORS origins: %+v", cfg.CORSAllowedOrigins)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.LogLevel)
	}
	if cfg.Environment != "production" {
		t.Errorf("expected env production, got %s", cfg.Environment)
	}
	if cfg.MigrateOnStartup {
		t.Errorf("expected MigrateOnStartup to be false")
	}
}
