package config

import (
	"fmt"
	"os"
)

// Config holds all runtime configuration, sourced from environment
// variables with development-friendly defaults.
type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	// AllowedOrigins for CORS (web dev servers).
	AllowedOrigins []string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:        getenv("SHANJI_PORT", "8090"),
		DatabaseURL: getenv("SHANJI_DATABASE_URL", "postgres://shanji:shanji@localhost:5433/shanji?sslmode=disable"),
		JWTSecret:   getenv("SHANJI_JWT_SECRET", "dev-secret-change-me"),
		AllowedOrigins: []string{
			"http://localhost:3000",
			"http://localhost:3002",
			"http://127.0.0.1:3000",
			"http://127.0.0.1:3002",
		},
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("SHANJI_JWT_SECRET must not be empty")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
