package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration loaded from environment variables and .env
type Config struct {
	Port                string
	DatabaseURL         string
	CORSAllowedOrigins  []string
	LogLevel            string
	Environment         string
	MigrateOnStartup    bool
}

// Load loads configuration from environment, with optional .env loading.
func Load() *Config {
	// Try loading .env from current dir or parent dirs if present
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	port := getEnv("PORT", getEnv("SERVER_PORT", "8080"))
	dbURL := getEnv("DATABASE_URL", "postgres://emailauto:emailauto@localhost:5432/emailauto?sslmode=disable")
	originsStr := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")
	logLevel := strings.ToLower(getEnv("LOG_LEVEL", "info"))
	environment := strings.ToLower(getEnv("ENV", getEnv("APP_ENV", "development")))
	migrateOnStartup := getEnvBool("MIGRATE_ON_STARTUP", true)

	var origins []string
	for _, o := range strings.Split(originsStr, ",") {
		trimmed := strings.TrimSpace(o)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	if len(origins) == 0 {
		origins = []string{"http://localhost:5173", "http://127.0.0.1:5173"}
	}

	return &Config{
		Port:               port,
		DatabaseURL:        dbURL,
		CORSAllowedOrigins: origins,
		LogLevel:           logLevel,
		Environment:        environment,
		MigrateOnStartup:   migrateOnStartup,
	}
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(val))
		if err == nil {
			return parsed
		}
	}
	return fallback
}
