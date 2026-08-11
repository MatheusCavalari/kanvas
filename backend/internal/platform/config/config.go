package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	DatabaseURL       string
	JWTSecret         string
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	SecureCookies     bool
	MigrationsPath    string
	CORSAllowedOrigin string
}

func Load() (Config, error) {
	cfg := Config{
		Port:              getEnv("PORT", "8080"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		JWTSecret:         os.Getenv("JWT_SECRET"),
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   7 * 24 * time.Hour,
		SecureCookies:     getEnv("SECURE_COOKIES", "false") == "true",
		MigrationsPath:    getEnv("MIGRATIONS_PATH", "db/migrations"),
		CORSAllowedOrigin: getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:5173"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	if v := os.Getenv("ACCESS_TOKEN_TTL_MINUTES"); v != "" {
		minutes, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL_MINUTES: %w", err)
		}
		cfg.AccessTokenTTL = time.Duration(minutes) * time.Minute
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
