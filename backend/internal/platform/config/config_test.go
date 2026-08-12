package config

import "testing"

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "secret")

	_, err := Load()

	if err == nil {
		t.Fatal("expected an error when DATABASE_URL is missing")
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("JWT_SECRET", "")

	_, err := Load()

	if err == nil {
		t.Fatal("expected an error when JWT_SECRET is missing")
	}
}

func TestLoad_DefaultsAndOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("PORT", "")
	t.Setenv("SECURE_COOKIES", "true")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Fatalf("expected default port 8080, got %q", cfg.Port)
	}
	if !cfg.SecureCookies {
		t.Fatal("expected SecureCookies to be true")
	}
	if cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= 0 {
		t.Fatal("expected positive default TTLs")
	}
}

func TestLoad_CORSAllowedOriginDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("CORS_ALLOWED_ORIGIN", "")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CORSAllowedOrigin != "http://localhost:5173" {
		t.Fatalf("expected default CORS origin http://localhost:5173, got %q", cfg.CORSAllowedOrigin)
	}
}

func TestLoad_CORSAllowedOriginOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("CORS_ALLOWED_ORIGIN", "https://kanvas.example.com")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CORSAllowedOrigin != "https://kanvas.example.com" {
		t.Fatalf("expected overridden CORS origin, got %q", cfg.CORSAllowedOrigin)
	}
}
