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
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GEMINI_MODEL")
	os.Unsetenv("GEMINI_RPM")
	os.Unsetenv("GEMINI_TIMEOUT_SECONDS")
	os.Unsetenv("GOOGLE_CLIENT_ID")
	os.Unsetenv("GOOGLE_CLIENT_SECRET")
	os.Unsetenv("GOOGLE_REDIRECT_URL")
	os.Unsetenv("ENCRYPTION_KEY")
	os.Unsetenv("FRONTEND_URL")

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
	if cfg.GeminiModel != "gemini-2.5-flash" {
		t.Errorf("expected default gemini model gemini-2.5-flash, got %s", cfg.GeminiModel)
	}
	if cfg.GoogleRedirectURL != "http://localhost:8080/auth/callback" {
		t.Errorf("expected default google redirect url, got %s", cfg.GoogleRedirectURL)
	}
	if cfg.EncryptionKey == "" {
		t.Errorf("expected default encryption key")
	}
}

func TestConfigCustomEnv(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("DATABASE_URL", "postgres://custom:custom@localhost:5433/custom?sslmode=disable")
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://frontend.local:3000,http://another.local")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ENV", "production")
	t.Setenv("MIGRATE_ON_STARTUP", "false")
	t.Setenv("GEMINI_API_KEY", "test-api-key")
	t.Setenv("GEMINI_MODEL", "gemini-1.5-flash")
	t.Setenv("GEMINI_RPM", "60")
	t.Setenv("GEMINI_TIMEOUT_SECONDS", "45")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/custom/callback")
	t.Setenv("ENCRYPTION_KEY", "custom-secret-key-1234567890123456")
	t.Setenv("FRONTEND_URL", "http://localhost:3000")

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
	if cfg.GoogleClientID != "test-client-id" {
		t.Errorf("expected google client id 'test-client-id', got %s", cfg.GoogleClientID)
	}
	if cfg.GoogleClientSecret != "test-secret" {
		t.Errorf("expected google client secret 'test-secret', got %s", cfg.GoogleClientSecret)
	}
	if cfg.GoogleRedirectURL != "http://localhost:8080/custom/callback" {
		t.Errorf("expected google redirect url, got %s", cfg.GoogleRedirectURL)
	}
}
