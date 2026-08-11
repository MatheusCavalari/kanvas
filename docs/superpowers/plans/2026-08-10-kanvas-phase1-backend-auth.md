# Kanvas — Phase 1: Backend Foundation + Auth — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working, tested REST API (`register`, `login`, `refresh`, `logout`, `me`) backed by PostgreSQL, runnable via `docker compose up`, with CI running lint and tests on every push.

**Architecture:** Clean/Hexagonal Go backend under `backend/`. Each domain package (`internal/auth`) has `domain.go` (entities/errors), `repository.go` (interface) + `repository_postgres.go` (sqlc-backed implementation), `service.go` (business logic behind interfaces), and `handler.go` (chi HTTP handlers). Shared infrastructure (config, DB pool, JWT, HTTP middleware, router) lives in `internal/platform`.

**Tech Stack:** Go 1.23, [chi](https://github.com/go-chi/chi) router, [pgx v5](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev/) (no ORM), [golang-migrate](https://github.com/golang-migrate/migrate), [golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt), bcrypt, [testcontainers-go](https://golang.testcontainers.org/) for integration tests, [testify](https://github.com/stretchr/testify), golangci-lint, Docker, GitHub Actions.

## Global Constraints

- Go module path: `github.com/MatheusCavalari/kanvas/backend` (repo will live at `github.com/MatheusCavalari/kanvas`).
- Go version: 1.23.
- HTTP router: chi. Data access: sqlc + pgx v5, hand-written SQL, no ORM.
- Migrations: golang-migrate, files under `backend/db/migrations`, numbered `NNNNNN_description.up.sql` / `.down.sql`.
- Auth: JWT access token (~15 min TTL) signed HS256, returned in the response body; refresh token (~7 days TTL) as a random opaque token, stored **hashed** (SHA-256) in Postgres, returned to the client only via an `httpOnly` cookie scoped to `/auth`.
- Passwords hashed with bcrypt (`bcrypt.DefaultCost`).
- Tests: unit tests use in-memory fakes (no `//go:build integration` tag). Integration/e2e tests use `testcontainers-go` against a real Postgres and are gated behind `//go:build integration` so `go test ./...` stays fast by default.
- Docker: multi-stage `Dockerfile` producing a minimal image. `docker-compose.yml` at the repo root for local dev.
- CI: GitHub Actions, path-scoped to `backend/**`, running lint + unit tests + integration tests.
- Repository: monorepo at `github.com/MatheusCavalari/kanvas`, committed incrementally (one commit per task step group), pushed to GitHub as development progresses — **but creating the remote repo and pushing requires explicit user confirmation of name/visibility at that time** (see Task 17).

---

## Task Overview

1. Backend scaffold + `/healthz`
2. Docker Compose (Postgres) + Makefile + env files + `.gitignore`
3. Config loader
4. Postgres connection pool
5. Migrations: `users`, `refresh_tokens`
6. sqlc setup + generated queries
7. JWT issuer
8. Auth domain types + repository interface + test fakes
9. Auth service: `Register`
10. Auth service: `Login`
11. Auth service: `Refresh`, `Logout`, `UserByID`
12. Postgres repository implementation (integration-tested)
13. JWT auth HTTP middleware
14. Auth HTTP handlers + routes
15. Wire `main.go` end-to-end + full-flow integration test
16. golangci-lint + GitHub Actions CI
17. Dockerfile + docker-compose backend service + README + push to GitHub

---

### Task 1: Backend scaffold + health check

**Files:**
- Create: `backend/go.mod`
- Create: `backend/cmd/api/main.go`
- Create: `backend/internal/platform/httpserver/server.go`
- Test: `backend/internal/platform/httpserver/server_test.go`

**Interfaces:**
- Produces: `httpserver.NewRouter() chi.Router` — mounts `GET /healthz` returning `200 {"status":"ok"}`. Returned type satisfies `http.Handler` (chi.Router embeds it) so it can be passed straight to `http.ListenAndServe`.

- [ ] **Step 1: Initialize the Go module**

Run (from repo root):
```bash
mkdir -p backend/cmd/api backend/internal/platform/httpserver
cd backend
go mod init github.com/MatheusCavalari/kanvas/backend
```

- [ ] **Step 2: Add the chi dependency**

Run (from `backend/`):
```bash
go get github.com/go-chi/chi/v5@latest
```

- [ ] **Step 3: Write the failing test**

`backend/internal/platform/httpserver/server_test.go`:
```go
package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	router := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf(`expected {"status":"ok"}, got %v`, body)
	}
}
```

- [ ] **Step 4: Run the test and confirm it fails**

Run (from `backend/`): `go test ./internal/platform/httpserver/... -v`
Expected: FAIL — `NewRouter` is undefined.

- [ ] **Step 5: Implement the router**

`backend/internal/platform/httpserver/server.go`:
```go
package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func NewRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(chimiddleware.Logger, chimiddleware.Recoverer)
	r.Get("/healthz", healthHandler)
	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 6: Run the test and confirm it passes**

Run (from `backend/`): `go test ./internal/platform/httpserver/... -v`
Expected: PASS

- [ ] **Step 7: Wire up `main.go`**

`backend/cmd/api/main.go`:
```go
package main

import (
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
)

func main() {
	router := httpserver.NewRouter()
	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 8: Verify the whole module builds**

Run (from `backend/`): `go build ./...`
Expected: no output, exit code 0.

- [ ] **Step 9: Commit**

```bash
git add backend/go.mod backend/go.sum backend/cmd backend/internal
git commit -m "feat(backend): scaffold Go module with health check endpoint"
```

---

### Task 2: Docker Compose, Makefile, env files, .gitignore

**Files:**
- Create: `docker-compose.yml` (repo root)
- Create: `backend/.env.example`
- Create: `backend/Makefile`
- Create: `.gitignore` (repo root)

**Interfaces:**
- Produces: `make -C backend migrate-up|migrate-down|sqlc-generate|test|test-integration|lint|run` — used by every subsequent task.

This task is infrastructure-only (no Go code), so it's verified by manually confirming the Postgres container comes up healthy rather than an automated test.

- [ ] **Step 1: Create the root `.gitignore`**

`.gitignore`:
```
# Go
backend/bin/
*.exe
*.test

# Env
**/.env
!**/.env.example

# Editors / OS
.DS_Store
.idea/
*.swp

# Frontend (future phases)
node_modules/
dist/
```

- [ ] **Step 2: Create `docker-compose.yml`**

`docker-compose.yml`:
```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: kanvas
      POSTGRES_PASSWORD: kanvas
      POSTGRES_DB: kanvas
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kanvas"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
```

- [ ] **Step 3: Create `backend/.env.example` and copy it to `backend/.env`**

`backend/.env.example`:
```
DATABASE_URL=postgres://kanvas:kanvas@localhost:5432/kanvas?sslmode=disable
JWT_SECRET=dev-secret-change-me
PORT=8080
SECURE_COOKIES=false
```

Run:
```bash
cp backend/.env.example backend/.env
```

- [ ] **Step 4: Create `backend/Makefile`**

`backend/Makefile`:
```makefile
include .env
export

MIGRATE := go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.1
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0

.PHONY: migrate-up migrate-down sqlc-generate test test-integration lint run

migrate-up:
	$(MIGRATE) -database "$(DATABASE_URL)" -path db/migrations up

migrate-down:
	$(MIGRATE) -database "$(DATABASE_URL)" -path db/migrations down 1

sqlc-generate:
	$(SQLC) generate

test:
	go test ./... -race

test-integration:
	go test ./... -race -tags=integration

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run

run:
	go run ./cmd/api
```

- [ ] **Step 5: Start Postgres and verify it's healthy**

Run (from repo root):
```bash
docker compose up -d postgres
docker compose ps
```
Expected: the `postgres` service shows state `running (healthy)`.

- [ ] **Step 6: Commit**

```bash
git add .gitignore docker-compose.yml backend/.env.example backend/Makefile
git commit -m "chore(backend): add docker-compose Postgres, Makefile, and env template"
```

---

### Task 3: Config loader

**Files:**
- Create: `backend/internal/platform/config/config.go`
- Test: `backend/internal/platform/config/config_test.go`
- Modify: `backend/cmd/api/main.go` (use `config.Load()` for the port)

**Interfaces:**
- Produces: `config.Config{Port, DatabaseURL, JWTSecret, AccessTokenTTL, RefreshTokenTTL, SecureCookies}`, `config.Load() (Config, error)`.

- [ ] **Step 1: Write the failing test**

`backend/internal/platform/config/config_test.go`:
```go
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
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `backend/`): `go test ./internal/platform/config/... -v`
Expected: FAIL — package `config` / `Load` undefined.

- [ ] **Step 3: Implement the config loader**

`backend/internal/platform/config/config.go`:
```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port            string
	DatabaseURL     string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	SecureCookies   bool
}

func Load() (Config, error) {
	cfg := Config{
		Port:            getEnv("PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       os.Getenv("JWT_SECRET"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
		SecureCookies:   getEnv("SECURE_COOKIES", "false") == "true",
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
```

- [ ] **Step 4: Run the test and confirm it passes**

Run (from `backend/`): `go test ./internal/platform/config/... -v`
Expected: PASS

- [ ] **Step 5: Use the config in `main.go`**

`backend/cmd/api/main.go`:
```go
package main

import (
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	router := httpserver.NewRouter()
	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 6: Verify the module still builds**

Run (from `backend/`): `go build ./...`

- [ ] **Step 7: Commit**

```bash
git add backend/internal/platform/config backend/cmd/api/main.go
git commit -m "feat(backend): add environment-based config loader"
```

---

### Task 4: Postgres connection pool

**Files:**
- Create: `backend/internal/platform/db/db.go`
- Test: `backend/internal/platform/db/db_test.go`

**Interfaces:**
- Produces: `db.NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)`.

- [ ] **Step 1: Add the pgx dependency**

Run (from `backend/`):
```bash
go get github.com/jackc/pgx/v5@latest
```

- [ ] **Step 2: Write the failing test**

`backend/internal/platform/db/db_test.go`:
```go
package db

import (
	"context"
	"testing"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	_, err := NewPool(context.Background(), "postgres://user:pass@bad host:5432/db")

	if err == nil {
		t.Fatal("expected an error for a malformed DSN")
	}
}
```

- [ ] **Step 3: Run the test and confirm it fails**

Run (from `backend/`): `go test ./internal/platform/db/... -v`
Expected: FAIL — `NewPool` undefined.

- [ ] **Step 4: Implement `NewPool`**

`backend/internal/platform/db/db.go`:
```go
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("creating pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 5: Run the test and confirm it passes**

Run (from `backend/`): `go test ./internal/platform/db/... -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/platform/db backend/go.mod backend/go.sum
git commit -m "feat(backend): add Postgres connection pool helper"
```

---

### Task 5: Migrations — `users`, `refresh_tokens`

**Files:**
- Create: `backend/db/migrations/000001_create_users_table.up.sql`
- Create: `backend/db/migrations/000001_create_users_table.down.sql`
- Create: `backend/db/migrations/000002_create_refresh_tokens_table.up.sql`
- Create: `backend/db/migrations/000002_create_refresh_tokens_table.down.sql`

**Interfaces:**
- Produces: `users` table (`id uuid pk`, `name`, `email unique`, `password_hash`, `created_at`, `updated_at`) and `refresh_tokens` table (`id uuid pk`, `user_id fk`, `token_hash unique`, `expires_at`, `revoked_at nullable`, `created_at`) — consumed by Task 6's sqlc queries and Task 12's repository.

No Go code in this task — verified by actually running the migrations against the compose Postgres.

- [ ] **Step 1: Create the `users` table migration**

`backend/db/migrations/000001_create_users_table.up.sql`:
```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`backend/db/migrations/000001_create_users_table.down.sql`:
```sql
DROP TABLE IF EXISTS users;
```

- [ ] **Step 2: Create the `refresh_tokens` table migration**

`backend/db/migrations/000002_create_refresh_tokens_table.up.sql`:
```sql
CREATE TABLE refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
```

`backend/db/migrations/000002_create_refresh_tokens_table.down.sql`:
```sql
DROP TABLE IF EXISTS refresh_tokens;
```

- [ ] **Step 3: Run the migrations against the local Postgres**

Run (from `backend/`, with `docker compose up -d postgres` already running):
```bash
make migrate-up
```
Expected: output ending in something like `000002/u create_refresh_tokens_table (X.XXXXXXXms)`, no errors.

- [ ] **Step 4: Verify the tables exist**

Run:
```bash
docker compose exec postgres psql -U kanvas -d kanvas -c "\dt"
```
Expected: lists `users`, `refresh_tokens`, and `schema_migrations`.

- [ ] **Step 5: Commit**

```bash
git add backend/db/migrations
git commit -m "feat(backend): add users and refresh_tokens migrations"
```

---

### Task 6: sqlc setup + generated queries

**Files:**
- Create: `backend/sqlc.yaml`
- Create: `backend/db/queries/users.sql`
- Create: `backend/db/queries/refresh_tokens.sql`
- Create: `backend/internal/platform/db/gen/*.go` (generated by sqlc — do not hand-edit)

**Interfaces:**
- Produces: `gen.New(pool) *gen.Queries` with methods `CreateUser`, `GetUserByEmail`, `GetUserByID`, `CreateRefreshToken`, `GetRefreshTokenByHash`, `RevokeRefreshToken`, and structs `gen.User`, `gen.RefreshToken`, `gen.CreateUserParams`, `gen.CreateRefreshTokenParams` — consumed by Task 12's `PostgresRepository`.

- [ ] **Step 1: Create `backend/sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: "postgresql"
    queries: "db/queries"
    schema: "db/migrations"
    gen:
      go:
        package: "gen"
        out: "internal/platform/db/gen"
        sql_package: "pgx/v5"
        emit_json_tags: true
        emit_pointers_for_null_types: true
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
```

- [ ] **Step 2: Write the user queries**

`backend/db/queries/users.sql`:
```sql
-- name: CreateUser :one
INSERT INTO users (id, name, email, password_hash)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;
```

- [ ] **Step 3: Write the refresh token queries**

`backend/db/queries/refresh_tokens.sql`:
```sql
-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4);

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens WHERE token_hash = $1;

-- name: RevokeRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1;
```

- [ ] **Step 4: Generate the Go code**

Run (from `backend/`):
```bash
make sqlc-generate
```
Expected: `internal/platform/db/gen/` now contains `db.go`, `models.go`, `users.sql.go`, `refresh_tokens.sql.go` (exact filenames may vary slightly by sqlc version).

- [ ] **Step 5: Add the uuid dependency and verify everything builds**

Run (from `backend/`):
```bash
go get github.com/google/uuid@latest
go build ./...
```
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add backend/sqlc.yaml backend/db/queries backend/internal/platform/db/gen backend/go.mod backend/go.sum
git commit -m "feat(backend): generate type-safe queries with sqlc"
```

---

### Task 7: JWT issuer

**Files:**
- Create: `backend/internal/platform/jwt/jwt.go`
- Test: `backend/internal/platform/jwt/jwt_test.go`

**Interfaces:**
- Produces: `jwt.NewIssuer(secret string, ttl time.Duration) *Issuer`, `(*Issuer).IssueAccessToken(userID uuid.UUID) (string, error)`, `(*Issuer).ParseAccessToken(token string) (uuid.UUID, error)`, `jwt.ErrInvalidToken` — consumed by Task 11 (service), Task 13 (middleware), Task 15 (wiring).

- [ ] **Step 1: Add dependencies**

Run (from `backend/`):
```bash
go get github.com/golang-jwt/jwt/v5@latest
go get github.com/stretchr/testify@latest
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/platform/jwt/jwt_test.go`:
```go
package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestIssuer_IssueAndParse_RoundTrip(t *testing.T) {
	issuer := NewIssuer("test-secret", time.Hour)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	parsedID, err := issuer.ParseAccessToken(token)
	require.NoError(t, err)
	require.Equal(t, userID, parsedID)
}

func TestIssuer_ParseAccessToken_WrongSecret(t *testing.T) {
	issuer := NewIssuer("test-secret", time.Hour)
	other := NewIssuer("other-secret", time.Hour)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	_, err = other.ParseAccessToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}

func TestIssuer_ParseAccessToken_Expired(t *testing.T) {
	issuer := NewIssuer("test-secret", -time.Minute)
	userID := uuid.New()

	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	_, err = issuer.ParseAccessToken(token)
	require.ErrorIs(t, err, ErrInvalidToken)
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/platform/jwt/... -v`
Expected: FAIL — package doesn't compile (`NewIssuer` undefined).

- [ ] **Step 4: Implement the issuer**

`backend/internal/platform/jwt/jwt.go`:
```go
package jwt

import (
	"errors"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid or expired access token")

type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

type claims struct {
	jwtlib.RegisteredClaims
}

func (i *Issuer) IssueAccessToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	c := claims{
		RegisteredClaims: jwtlib.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwtlib.NewNumericDate(now),
			ExpiresAt: jwtlib.NewNumericDate(now.Add(i.ttl)),
		},
	}
	token := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, c)
	return token.SignedString(i.secret)
}

func (i *Issuer) ParseAccessToken(tokenString string) (uuid.UUID, error) {
	var c claims
	token, err := jwtlib.ParseWithClaims(tokenString, &c, func(t *jwtlib.Token) (interface{}, error) {
		return i.secret, nil
	})
	if err != nil || !token.Valid {
		return uuid.UUID{}, ErrInvalidToken
	}

	userID, err := uuid.Parse(c.Subject)
	if err != nil {
		return uuid.UUID{}, ErrInvalidToken
	}
	return userID, nil
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/platform/jwt/... -v`
Expected: PASS (3 tests)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/platform/jwt backend/go.mod backend/go.sum
git commit -m "feat(backend): add JWT access token issuer"
```

---

### Task 8: Auth domain types, repository interface, test fakes

**Files:**
- Create: `backend/internal/auth/domain.go`
- Create: `backend/internal/auth/repository.go`
- Create: `backend/internal/auth/repository_fake_test.go`
- Create: `backend/internal/auth/token_issuer_fake_test.go`
- Test: `backend/internal/auth/repository_fake_test.go` (contains a sanity-check test on the fake itself)

**Interfaces:**
- Produces: `auth.User`, `auth.RefreshToken`, domain errors (`ErrEmailTaken`, `ErrInvalidCredentials`, `ErrRefreshTokenInvalid`, `ErrUserNotFound`), `auth.Repository` interface, `auth.ErrNotFound` (repository-layer sentinel) — consumed by every later `internal/auth` task.

- [ ] **Step 1: Define domain types and errors**

`backend/internal/auth/domain.go`:
```go
package auth

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

var (
	ErrEmailTaken          = errors.New("email already registered")
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrRefreshTokenInvalid = errors.New("invalid or expired refresh token")
	ErrUserNotFound        = errors.New("user not found")
)
```

- [ ] **Step 2: Define the repository interface**

`backend/internal/auth/repository.go`:
```go
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateUser(ctx context.Context, u User) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (User, error)
	CreateRefreshToken(ctx context.Context, t RefreshToken) error
	GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id uuid.UUID) error
}
```

- [ ] **Step 3: Write the failing sanity test for the in-memory fake**

`backend/internal/auth/repository_fake_test.go`:
```go
package auth

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

type fakeRepository struct {
	mu            sync.Mutex
	usersByID     map[uuid.UUID]User
	usersByEmail  map[string]uuid.UUID
	refreshTokens map[uuid.UUID]RefreshToken
	tokensByHash  map[string]uuid.UUID
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		usersByID:     make(map[uuid.UUID]User),
		usersByEmail:  make(map[string]uuid.UUID),
		refreshTokens: make(map[uuid.UUID]RefreshToken),
		tokensByHash:  make(map[string]uuid.UUID),
	}
}

func (f *fakeRepository) CreateUser(ctx context.Context, u User) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usersByID[u.ID] = u
	f.usersByEmail[u.Email] = u.ID
	return u, nil
}

func (f *fakeRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.usersByEmail[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return f.usersByID[id], nil
}

func (f *fakeRepository) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.usersByID[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (f *fakeRepository) CreateRefreshToken(ctx context.Context, t RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshTokens[t.ID] = t
	f.tokensByHash[t.TokenHash] = t.ID
	return nil
}

func (f *fakeRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.tokensByHash[tokenHash]
	if !ok {
		return RefreshToken{}, ErrNotFound
	}
	return f.refreshTokens[id], nil
}

func (f *fakeRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.refreshTokens[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	t.RevokedAt = &now
	f.refreshTokens[id] = t
	return nil
}

func TestFakeRepository_CreateAndGetUser(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()

	created, err := repo.CreateUser(ctx, User{ID: uuid.New(), Name: "Ada", Email: "ada@example.com"})
	require.NoError(t, err)

	fetched, err := repo.GetUserByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)

	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 4: Run the test and confirm it fails, then passes**

Run (from `backend/`): `go test ./internal/auth/... -v`
First run: FAIL (package doesn't compile until the file above exists — this file *is* the implementation, so after saving it the test should already pass). Re-run to confirm PASS.

- [ ] **Step 5: Add the fake token issuer**

`backend/internal/auth/token_issuer_fake_test.go`:
```go
package auth

import "github.com/google/uuid"

type fakeTokenIssuer struct{}

func newFakeTokenIssuer() *fakeTokenIssuer {
	return &fakeTokenIssuer{}
}

func (f *fakeTokenIssuer) IssueAccessToken(userID uuid.UUID) (string, error) {
	return "access-" + userID.String(), nil
}
```

- [ ] **Step 6: Verify the module builds**

Run (from `backend/`): `go build ./... && go test ./internal/auth/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/auth
git commit -m "feat(backend): add auth domain types, repository interface, and test fakes"
```

---

### Task 9: Auth service — `Register`

**Files:**
- Create: `backend/internal/auth/service.go`
- Create: `backend/internal/auth/service_test.go`

**Interfaces:**
- Consumes: `auth.Repository` (Task 8), `auth.fakeRepository`/`auth.fakeTokenIssuer` (Task 8, test-only).
- Produces: `auth.TokenIssuer` interface (`IssueAccessToken(uuid.UUID) (string, error)`), `auth.Service`, `auth.NewService(repo Repository, tokens TokenIssuer, refreshTTL time.Duration) *Service`, `auth.AuthResult{User, AccessToken, RefreshToken, RefreshExpiresAt}`, `(*Service).Register(ctx, name, email, password string) (AuthResult, error)` — consumed by Task 10, 11, 14, 15.

- [ ] **Step 1: Add the bcrypt dependency**

Run (from `backend/`):
```bash
go get golang.org/x/crypto@latest
```

- [ ] **Step 2: Write the failing tests**

`backend/internal/auth/service_test.go`:
```go
package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestService_Register_Success(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)

	result, err := svc.Register(context.Background(), "Ada Lovelace", "ada@example.com", "supersecret")

	require.NoError(t, err)
	require.Equal(t, "ada@example.com", result.User.Email)
	require.NotEmpty(t, result.AccessToken)
	require.NotEmpty(t, result.RefreshToken)
	require.NotEqual(t, "supersecret", result.User.PasswordHash)
}

func TestService_Register_DuplicateEmail(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	_, err = svc.Register(ctx, "Someone Else", "ada@example.com", "otherpass")

	require.True(t, errors.Is(err, ErrEmailTaken))
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/auth/... -run TestService_Register -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 4: Implement the service with `Register`**

`backend/internal/auth/service.go`:
```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type TokenIssuer interface {
	IssueAccessToken(userID uuid.UUID) (string, error)
}

type Service struct {
	repo       Repository
	tokens     TokenIssuer
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(repo Repository, tokens TokenIssuer, refreshTTL time.Duration) *Service {
	return &Service{repo: repo, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

type AuthResult struct {
	User             User
	AccessToken      string
	RefreshToken     string
	RefreshExpiresAt time.Time
}

func (s *Service) Register(ctx context.Context, name, email, password string) (AuthResult, error) {
	if _, err := s.repo.GetUserByEmail(ctx, email); err == nil {
		return AuthResult{}, ErrEmailTaken
	} else if !errors.Is(err, ErrNotFound) {
		return AuthResult{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return AuthResult{}, err
	}

	user, err := s.repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         name,
		Email:        email,
		PasswordHash: string(hash),
	})
	if err != nil {
		return AuthResult{}, err
	}

	return s.issueAuthResult(ctx, user)
}

func (s *Service) issueAuthResult(ctx context.Context, user User) (AuthResult, error) {
	accessToken, err := s.tokens.IssueAccessToken(user.ID)
	if err != nil {
		return AuthResult{}, err
	}

	rawRefreshToken, err := generateRefreshToken()
	if err != nil {
		return AuthResult{}, err
	}

	expiresAt := s.now().Add(s.refreshTTL)
	if err := s.repo.CreateRefreshToken(ctx, RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: hashToken(rawRefreshToken),
		ExpiresAt: expiresAt,
	}); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:             user,
		AccessToken:      accessToken,
		RefreshToken:     rawRefreshToken,
		RefreshExpiresAt: expiresAt,
	}, nil
}

func generateRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/auth/... -run TestService_Register -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/auth backend/go.mod backend/go.sum
git commit -m "feat(backend): implement auth service Register"
```

---

### Task 10: Auth service — `Login`

**Files:**
- Modify: `backend/internal/auth/service.go` (add `Login`)
- Modify: `backend/internal/auth/service_test.go` (add tests)

**Interfaces:**
- Consumes: everything from Task 9's `service.go`.
- Produces: `(*Service).Login(ctx, email, password string) (AuthResult, error)` — consumed by Task 14, 15.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/auth/service_test.go`:
```go
func TestService_Login_Success(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	result, err := svc.Login(ctx, "ada@example.com", "supersecret")

	require.NoError(t, err)
	require.Equal(t, "ada@example.com", result.User.Email)
}

func TestService_Login_WrongPassword(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	_, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	_, err = svc.Login(ctx, "ada@example.com", "wrongpass")

	require.True(t, errors.Is(err, ErrInvalidCredentials))
}

func TestService_Login_UnknownEmail(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)

	_, err := svc.Login(context.Background(), "nobody@example.com", "whatever")

	require.True(t, errors.Is(err, ErrInvalidCredentials))
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/auth/... -run TestService_Login -v`
Expected: FAIL — `(*Service).Login` undefined.

- [ ] **Step 3: Implement `Login`**

Append to `backend/internal/auth/service.go`:
```go
func (s *Service) Login(ctx context.Context, email, password string) (AuthResult, error) {
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.issueAuthResult(ctx, user)
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/auth/... -run TestService_Login -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth/service.go backend/internal/auth/service_test.go
git commit -m "feat(backend): implement auth service Login"
```

---

### Task 11: Auth service — `Refresh`, `Logout`, `UserByID`

**Files:**
- Modify: `backend/internal/auth/service.go` (add `Refresh`, `Logout`, `UserByID`)
- Modify: `backend/internal/auth/service_test.go` (add tests)

**Interfaces:**
- Produces: `(*Service).Refresh(ctx, rawRefreshToken string) (AuthResult, error)`, `(*Service).Logout(ctx, rawRefreshToken string) error`, `(*Service).UserByID(ctx, id uuid.UUID) (User, error)` — consumed by Task 14, 15.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/auth/service_test.go`:
```go
func TestService_Refresh_RotatesToken(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	registered, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	refreshed, err := svc.Refresh(ctx, registered.RefreshToken)
	require.NoError(t, err)
	require.NotEqual(t, registered.RefreshToken, refreshed.RefreshToken)

	_, err = svc.Refresh(ctx, registered.RefreshToken)
	require.True(t, errors.Is(err, ErrRefreshTokenInvalid))
}

func TestService_Refresh_ExpiredToken(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	start := time.Now()
	svc.now = func() time.Time { return start }
	ctx := context.Background()

	registered, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	svc.now = func() time.Time { return start.Add(2 * time.Hour) }

	_, err = svc.Refresh(ctx, registered.RefreshToken)
	require.True(t, errors.Is(err, ErrRefreshTokenInvalid))
}

func TestService_Logout_RevokesToken(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)
	ctx := context.Background()

	registered, err := svc.Register(ctx, "Ada", "ada@example.com", "supersecret")
	require.NoError(t, err)

	err = svc.Logout(ctx, registered.RefreshToken)
	require.NoError(t, err)

	_, err = svc.Refresh(ctx, registered.RefreshToken)
	require.True(t, errors.Is(err, ErrRefreshTokenInvalid))
}

func TestService_UserByID_NotFound(t *testing.T) {
	repo := newFakeRepository()
	issuer := newFakeTokenIssuer()
	svc := NewService(repo, issuer, time.Hour)

	_, err := svc.UserByID(context.Background(), uuid.New())

	require.True(t, errors.Is(err, ErrUserNotFound))
}
```

Add `"github.com/google/uuid"` to the test file's imports.

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/auth/... -run "TestService_Refresh|TestService_Logout|TestService_UserByID" -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement `Refresh`, `Logout`, `UserByID`**

Append to `backend/internal/auth/service.go`:
```go
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (AuthResult, error) {
	hash := hashToken(rawRefreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, ErrRefreshTokenInvalid
		}
		return AuthResult{}, err
	}

	if stored.RevokedAt != nil || s.now().After(stored.ExpiresAt) {
		return AuthResult{}, ErrRefreshTokenInvalid
	}

	if err := s.repo.RevokeRefreshToken(ctx, stored.ID); err != nil {
		return AuthResult{}, err
	}

	user, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil {
		return AuthResult{}, err
	}

	return s.issueAuthResult(ctx, user)
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	hash := hashToken(rawRefreshToken)
	stored, err := s.repo.GetRefreshTokenByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.repo.RevokeRefreshToken(ctx, stored.ID)
}

func (s *Service) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, ErrUserNotFound
		}
		return User{}, err
	}
	return user, nil
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/auth/... -v`
Expected: PASS (all `Service` tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth/service.go backend/internal/auth/service_test.go
git commit -m "feat(backend): implement auth service Refresh, Logout, UserByID"
```

---

### Task 12: Postgres repository implementation (integration-tested)

**Files:**
- Create: `backend/internal/platform/db/dbtest/dbtest.go`
- Create: `backend/internal/auth/repository_postgres.go`
- Test: `backend/internal/auth/repository_postgres_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `gen.Queries` (Task 6), `auth.Repository` (Task 8).
- Produces: `dbtest.NewPool(t *testing.T) *pgxpool.Pool` (spins up a throwaway Postgres via testcontainers, runs migrations, returns a ready pool — reused by Task 15), `auth.NewPostgresRepository(q *gen.Queries) *PostgresRepository` implementing `auth.Repository`.

- [ ] **Step 1: Add testcontainers dependencies**

Run (from `backend/`):
```bash
go get github.com/testcontainers/testcontainers-go@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go get github.com/golang-migrate/migrate/v4@latest
```

- [ ] **Step 2: Implement the `dbtest` helper**

`backend/internal/platform/db/dbtest/dbtest.go`:
```go
package dbtest

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewPool starts a throwaway Postgres container, applies all migrations
// from backend/db/migrations, and returns a ready-to-use pool. The
// container and pool are torn down automatically via t.Cleanup.
func NewPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("kanvas_test"),
		tcpostgres.WithUsername("kanvas"),
		tcpostgres.WithPassword("kanvas"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("5432/tcp")),
	)
	if err != nil {
		t.Fatalf("starting postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("getting connection string: %v", err)
	}

	m, err := migrate.New("file://"+filepath.ToSlash(migrationsDir()), dsn)
	if err != nil {
		t.Fatalf("creating migrator: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("running migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// migrationsDir resolves backend/db/migrations relative to this source
// file, so it works no matter which package's test calls NewPool.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile: .../backend/internal/platform/db/dbtest/dbtest.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "db", "migrations")
}
```

- [ ] **Step 3: Write the failing integration test**

`backend/internal/auth/repository_postgres_test.go`:
```go
//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func TestPostgresRepository_CreateAndGetUser(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := NewPostgresRepository(gen.New(pool))
	ctx := context.Background()

	created, err := repo.CreateUser(ctx, User{
		ID:           uuid.New(),
		Name:         "Ada Lovelace",
		Email:        "ada@example.com",
		PasswordHash: "hashed",
	})
	require.NoError(t, err)
	require.Equal(t, "ada@example.com", created.Email)

	fetched, err := repo.GetUserByEmail(ctx, "ada@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, fetched.ID)

	_, err = repo.GetUserByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_RefreshTokenLifecycle(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := NewPostgresRepository(gen.New(pool))
	ctx := context.Background()

	user, err := repo.CreateUser(ctx, User{ID: uuid.New(), Name: "Ada", Email: "ada2@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	token := RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: "some-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	require.NoError(t, repo.CreateRefreshToken(ctx, token))

	fetched, err := repo.GetRefreshTokenByHash(ctx, "some-hash")
	require.NoError(t, err)
	require.Nil(t, fetched.RevokedAt)

	require.NoError(t, repo.RevokeRefreshToken(ctx, token.ID))

	fetched, err = repo.GetRefreshTokenByHash(ctx, "some-hash")
	require.NoError(t, err)
	require.NotNil(t, fetched.RevokedAt)
}
```

- [ ] **Step 4: Run the test and confirm it fails**

Run (from `backend/`): `make test-integration`
Expected: FAIL — `NewPostgresRepository` undefined. (Requires Docker running locally.)

- [ ] **Step 5: Implement the Postgres repository**

`backend/internal/auth/repository_postgres.go`:
```go
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateUser(ctx context.Context, u User) (User, error) {
	row, err := r.q.CreateUser(ctx, gen.CreateUserParams{
		ID:           u.ID,
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
	})
	if err != nil {
		return User{}, err
	}
	return toDomainUser(row), nil
}

func (r *PostgresRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return toDomainUser(row), nil
}

func (r *PostgresRepository) GetUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	return toDomainUser(row), nil
}

func (r *PostgresRepository) CreateRefreshToken(ctx context.Context, t RefreshToken) error {
	return r.q.CreateRefreshToken(ctx, gen.CreateRefreshTokenParams{
		ID:        t.ID,
		UserID:    t.UserID,
		TokenHash: t.TokenHash,
		ExpiresAt: t.ExpiresAt,
	})
}

func (r *PostgresRepository) GetRefreshTokenByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	row, err := r.q.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefreshToken{}, ErrNotFound
		}
		return RefreshToken{}, err
	}
	return toDomainRefreshToken(row), nil
}

func (r *PostgresRepository) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return r.q.RevokeRefreshToken(ctx, id)
}

func toDomainUser(row gen.User) User {
	return User{
		ID:           row.ID,
		Name:         row.Name,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func toDomainRefreshToken(row gen.RefreshToken) RefreshToken {
	return RefreshToken{
		ID:        row.ID,
		UserID:    row.UserID,
		TokenHash: row.TokenHash,
		ExpiresAt: row.ExpiresAt,
		RevokedAt: row.RevokedAt,
		CreatedAt: row.CreatedAt,
	}
}
```

- [ ] **Step 6: Run the test and confirm it passes**

Run (from `backend/`): `make test-integration`
Expected: PASS (requires Docker running locally; the first run pulls the `postgres:16-alpine` image).

- [ ] **Step 7: Commit**

```bash
git add backend/internal/platform/db/dbtest backend/internal/auth/repository_postgres.go backend/internal/auth/repository_postgres_test.go backend/go.mod backend/go.sum
git commit -m "feat(backend): implement Postgres-backed auth repository with integration tests"
```

---

### Task 13: JWT auth HTTP middleware

**Files:**
- Create: `backend/internal/platform/middleware/auth.go`
- Test: `backend/internal/platform/middleware/auth_test.go`

**Interfaces:**
- Consumes: `jwt.Issuer` (Task 7).
- Produces: `middleware.Auth(issuer *jwt.Issuer) func(http.Handler) http.Handler`, `middleware.UserIDFromContext(ctx context.Context) (uuid.UUID, bool)` — consumed by Task 14, 15.

- [ ] **Step 1: Write the failing tests**

`backend/internal/platform/middleware/auth_test.go`:
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
)

func TestAuth_ValidToken(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	userID := uuid.New()
	token, err := issuer.IssueAccessToken(userID)
	require.NoError(t, err)

	var gotID uuid.UUID
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, _ = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, userID, gotID)
}

func TestAuth_MissingHeader(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_InvalidToken(t *testing.T) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()

	Auth(issuer)(next).ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/platform/middleware/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement the middleware**

`backend/internal/platform/middleware/auth.go`:
```go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
)

type contextKey string

const userIDContextKey contextKey = "userID"

func Auth(issuer *jwt.Issuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			token, ok := strings.CutPrefix(header, "Bearer ")
			if !ok || token == "" {
				http.Error(w, "missing or invalid Authorization header", http.StatusUnauthorized)
				return
			}

			userID, err := issuer.ParseAccessToken(token)
			if err != nil {
				http.Error(w, "invalid or expired access token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return id, ok
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/platform/middleware/... -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/platform/middleware
git commit -m "feat(backend): add JWT auth HTTP middleware"
```

---

### Task 14: Auth HTTP handlers + routes

**Files:**
- Create: `backend/internal/auth/handler.go`
- Test: `backend/internal/auth/handler_test.go`

**Interfaces:**
- Consumes: `AuthResult`, `User`, `ErrEmailTaken`, `ErrInvalidCredentials`, `ErrRefreshTokenInvalid` (Tasks 8-11), `middleware.UserIDFromContext` (Task 13).
- Produces: `auth.NewHandler(service authService, secureCookies bool) *Handler`, `(*Handler).RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler)` mounting `POST /auth/register|login|refresh|logout` and `GET /auth/me` (protected) — consumed by Task 15.

- [ ] **Step 1: Add the chi dependency to this package's imports (already in go.mod from Task 1) and write the failing tests**

`backend/internal/auth/handler_test.go`:
```go
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeAuthService struct {
	registerFn func(ctx context.Context, name, email, password string) (AuthResult, error)
	loginFn    func(ctx context.Context, email, password string) (AuthResult, error)
	refreshFn  func(ctx context.Context, token string) (AuthResult, error)
	logoutFn   func(ctx context.Context, token string) error
	userByIDFn func(ctx context.Context, id uuid.UUID) (User, error)
}

func (f *fakeAuthService) Register(ctx context.Context, name, email, password string) (AuthResult, error) {
	return f.registerFn(ctx, name, email, password)
}
func (f *fakeAuthService) Login(ctx context.Context, email, password string) (AuthResult, error) {
	return f.loginFn(ctx, email, password)
}
func (f *fakeAuthService) Refresh(ctx context.Context, token string) (AuthResult, error) {
	return f.refreshFn(ctx, token)
}
func (f *fakeAuthService) Logout(ctx context.Context, token string) error {
	return f.logoutFn(ctx, token)
}
func (f *fakeAuthService) UserByID(ctx context.Context, id uuid.UUID) (User, error) {
	return f.userByIDFn(ctx, id)
}

func passthroughMiddleware(next http.Handler) http.Handler { return next }

func TestHandler_Register_Success(t *testing.T) {
	svc := &fakeAuthService{
		registerFn: func(ctx context.Context, name, email, password string) (AuthResult, error) {
			return AuthResult{
				User:             User{ID: uuid.New(), Name: name, Email: email},
				AccessToken:      "access-token",
				RefreshToken:     "refresh-token",
				RefreshExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	h.RegisterRoutes(r, passthroughMiddleware)

	body, _ := json.Marshal(registerRequest{Name: "Ada", Email: "ada@example.com", Password: "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp authResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "access-token", resp.AccessToken)
	require.Equal(t, "ada@example.com", resp.User.Email)

	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "refresh_token", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
}

func TestHandler_Register_DuplicateEmail(t *testing.T) {
	svc := &fakeAuthService{
		registerFn: func(ctx context.Context, name, email, password string) (AuthResult, error) {
			return AuthResult{}, ErrEmailTaken
		},
	}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	h.RegisterRoutes(r, passthroughMiddleware)

	body, _ := json.Marshal(registerRequest{Name: "Ada", Email: "ada@example.com", Password: "supersecret"})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestHandler_Me_RequiresAuthMiddleware(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewHandler(svc, false)
	r := chi.NewRouter()
	blockAll := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
	h.RegisterRoutes(r, blockAll)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/auth/... -run TestHandler -v`
Expected: FAIL — `NewHandler` undefined.

- [ ] **Step 3: Implement the handlers**

`backend/internal/auth/handler.go`:
```go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

const refreshCookieName = "refresh_token"

type authService interface {
	Register(ctx context.Context, name, email, password string) (AuthResult, error)
	Login(ctx context.Context, email, password string) (AuthResult, error)
	Refresh(ctx context.Context, rawRefreshToken string) (AuthResult, error)
	Logout(ctx context.Context, rawRefreshToken string) error
	UserByID(ctx context.Context, id uuid.UUID) (User, error)
}

type Handler struct {
	service       authService
	secureCookies bool
}

func NewHandler(service authService, secureCookies bool) *Handler {
	return &Handler{service: service, secureCookies: secureCookies}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.Register)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.With(authMiddleware).Get("/me", h.Me)
	})
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	AccessToken string   `json:"access_token"`
	User        userView `json:"user"`
}

type userView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "name, email and password are required", http.StatusBadRequest)
		return
	}

	result, err := h.service.Register(r.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.respondWithAuthResult(w, result, http.StatusCreated)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.service.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.respondWithAuthResult(w, result, http.StatusOK)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil {
		http.Error(w, "missing refresh token", http.StatusUnauthorized)
		return
	}

	result, err := h.service.Refresh(r.Context(), cookie.Value)
	if err != nil {
		h.writeAuthError(w, err)
		return
	}

	h.respondWithAuthResult(w, result, http.StatusOK)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.service.UserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, userView{ID: user.ID.String(), Name: user.Name, Email: user.Email})
}

func (h *Handler) respondWithAuthResult(w http.ResponseWriter, result AuthResult, status int) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    result.RefreshToken,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  result.RefreshExpiresAt,
	})

	writeJSON(w, status, authResponse{
		AccessToken: result.AccessToken,
		User: userView{
			ID:    result.User.ID.String(),
			Name:  result.User.Name,
			Email: result.User.Email,
		},
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmailTaken):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, ErrInvalidCredentials):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, ErrRefreshTokenInvalid):
		http.Error(w, err.Error(), http.StatusUnauthorized)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/auth/... -v`
Expected: PASS (all `auth` package unit tests, including the new handler tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/auth/handler.go backend/internal/auth/handler_test.go
git commit -m "feat(backend): add auth HTTP handlers and routes"
```

---

### Task 15: Wire `main.go` end-to-end + full-flow integration test

**Files:**
- Modify: `backend/cmd/api/main.go`
- Create: `backend/internal/auth/e2e_test.go` (build tag `integration`, package `auth_test`)

**Interfaces:**
- Consumes: everything produced by Tasks 3, 4, 7, 12, 13, 14.
- Produces: a fully wired running server — nothing new for later Phase-1 tasks, but this is what Phase 2 will build on.

- [ ] **Step 1: Write the failing end-to-end test**

`backend/internal/auth/e2e_test.go`:
```go
//go:build integration

package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestAuthFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	repo := auth.NewPostgresRepository(gen.New(pool))
	issuer := jwt.NewIssuer("test-secret", 15*time.Minute)
	service := auth.NewService(repo, issuer, 7*24*time.Hour)
	handler := auth.NewHandler(service, false)

	router := httpserver.NewRouter()
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	registerBody, _ := json.Marshal(map[string]string{
		"name": "Ada Lovelace", "email": "ada@example.com", "password": "supersecret",
	})
	resp, err := client.Post(server.URL+"/auth/register", "application/json", bytes.NewReader(registerBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var registerResp struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&registerResp))
	require.NotEmpty(t, registerResp.AccessToken)

	var refreshCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "refresh_token" {
			refreshCookie = c
		}
	}
	require.NotNil(t, refreshCookie)

	meReq, _ := http.NewRequest(http.MethodGet, server.URL+"/auth/me", nil)
	meReq.Header.Set("Authorization", "Bearer "+registerResp.AccessToken)
	meResp, err := client.Do(meReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, meResp.StatusCode)

	refreshReq, _ := http.NewRequest(http.MethodPost, server.URL+"/auth/refresh", nil)
	refreshReq.AddCookie(refreshCookie)
	refreshResp, err := client.Do(refreshReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, refreshResp.StatusCode)

	logoutReq, _ := http.NewRequest(http.MethodPost, server.URL+"/auth/logout", nil)
	logoutReq.AddCookie(refreshCookie)
	logoutResp, err := client.Do(logoutReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, logoutResp.StatusCode)
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `backend/`): `make test-integration`
Expected: FAIL — `httpserver.NewRouter()` currently takes no auth handler wiring at the call site, but more importantly this exercises real code paths already implemented in Tasks 7-14, so it should mostly compile; if it doesn't compile yet it's because `main.go` hasn't been touched — that's fine, this test doesn't depend on `main.go`. If it fails, read the failure output before proceeding (it should point at a real logic gap, not a missing symbol, since everything it calls already exists from prior tasks).

- [ ] **Step 3: Wire `main.go` completely**

`backend/cmd/api/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"

	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	repo := auth.NewPostgresRepository(gen.New(pool))
	issuer := jwt.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	service := auth.NewService(repo, issuer, cfg.RefreshTokenTTL)
	handler := auth.NewHandler(service, cfg.SecureCookies)

	router := httpserver.NewRouter()
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
```

- [ ] **Step 4: Run the integration test and confirm it passes**

Run (from `backend/`): `make test-integration`
Expected: PASS

- [ ] **Step 5: Smoke-test the real server manually**

Run (from `backend/`, with `docker compose up -d postgres` and migrations applied):
```bash
make run
```
In another terminal:
```bash
curl -i -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Ada Lovelace","email":"ada@example.com","password":"supersecret"}'
```
Expected: `201 Created` with an `access_token` in the JSON body and a `Set-Cookie: refresh_token=...` header. Stop the server with Ctrl+C.

- [ ] **Step 6: Commit**

```bash
git add backend/cmd/api/main.go backend/internal/auth/e2e_test.go
git commit -m "feat(backend): wire main.go end-to-end and add auth flow integration test"
```

---

### Task 16: golangci-lint + GitHub Actions CI

**Files:**
- Create: `backend/.golangci.yml`
- Create: `.github/workflows/backend-ci.yml`

**Interfaces:** none (CI configuration only).

- [ ] **Step 1: Add the lint config**

`backend/.golangci.yml`:
```yaml
run:
  timeout: 3m

linters:
  enable:
    - govet
    - staticcheck
    - errcheck
    - unused
    - gofmt
    - goimports
```

- [ ] **Step 2: Run lint locally and fix any findings**

Run (from `backend/`): `make lint`
Expected: no issues (fix anything it reports before moving on).

- [ ] **Step 3: Add the CI workflow**

`.github/workflows/backend-ci.yml`:
```yaml
name: backend-ci

on:
  push:
    paths:
      - "backend/**"
      - ".github/workflows/backend-ci.yml"
  pull_request:
    paths:
      - "backend/**"
      - ".github/workflows/backend-ci.yml"

jobs:
  lint:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: backend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache-dependency-path: backend/go.sum
      - uses: golangci/golangci-lint-action@v6
        with:
          working-directory: backend
          version: latest

  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: backend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.23"
          cache-dependency-path: backend/go.sum
      - run: go build ./...
      - run: go test ./... -race -v
      - run: go test ./... -race -tags=integration -v
```

- [ ] **Step 4: Verify the workflow YAML is well-formed**

Run (from repo root):
```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/backend-ci.yml'))" 2>/dev/null || echo "install a YAML linter or visually re-check indentation"
```
(This step just guards against a typo before pushing; the real verification happens once this workflow runs on GitHub in Task 17.)

- [ ] **Step 5: Commit**

```bash
git add backend/.golangci.yml .github/workflows/backend-ci.yml
git commit -m "chore(backend): add golangci-lint config and GitHub Actions CI"
```

---

### Task 17: Dockerfile, docker-compose backend service, README, push to GitHub

**Files:**
- Create: `backend/Dockerfile`
- Modify: `docker-compose.yml` (add the `backend` service)
- Create: `backend/README.md`
- Create: `README.md` (repo root)

**Interfaces:** none (packaging, docs, and publishing only).

- [ ] **Step 1: Write the Dockerfile**

`backend/Dockerfile`:
```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api

FROM alpine:3.20
RUN adduser -D -u 10001 appuser
COPY --from=build /out/api /usr/local/bin/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/api"]
```

- [ ] **Step 2: Add the `backend` service to `docker-compose.yml`**

`docker-compose.yml` (full file):
```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: kanvas
      POSTGRES_PASSWORD: kanvas
      POSTGRES_DB: kanvas
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kanvas"]
      interval: 5s
      timeout: 5s
      retries: 5

  backend:
    build: ./backend
    environment:
      DATABASE_URL: postgres://kanvas:kanvas@postgres:5432/kanvas?sslmode=disable
      JWT_SECRET: dev-secret-change-me
      PORT: "8080"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  postgres_data:
```

- [ ] **Step 3: Build and run the full stack, then verify manually**

Run (from repo root):
```bash
docker compose up -d --build
```
Wait a few seconds, then run the migrations against the now-running container Postgres (from `backend/`, using the host-side `.env` which points at `localhost:5432`):
```bash
make migrate-up
```
Then:
```bash
curl -i -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Ada Lovelace","email":"ada2@example.com","password":"supersecret"}'
```
Expected: `201 Created`. Tear down with `docker compose down`.

- [ ] **Step 4: Write `backend/README.md`**

`backend/README.md`:
```markdown
# Kanvas backend

Go REST API for Kanvas — Clean/Hexagonal architecture, chi router, sqlc + pgx, JWT auth.

## Prerequisites

- Go 1.23+
- Docker (for local Postgres and for integration tests via testcontainers)

## Local development

    cp .env.example .env
    docker compose up -d postgres   # from the repo root
    make migrate-up
    make sqlc-generate
    make run

The API listens on `:8080` (see `.env`). Try it:

    curl -X POST http://localhost:8080/auth/register \
      -H "Content-Type: application/json" \
      -d '{"name":"Ada Lovelace","email":"ada@example.com","password":"supersecret"}'

## Tests

    make test              # unit tests (fast, no Docker required)
    make test-integration  # integration + e2e tests (spins up Postgres via testcontainers)

## Project layout

    cmd/api/          entry point
    internal/auth/    auth domain, service, repository, HTTP handlers
    internal/platform/ shared infra: config, db, jwt, middleware, http router
    db/migrations/    golang-migrate SQL migrations
    db/queries/       sqlc source queries
```

- [ ] **Step 5: Write the root `README.md`**

`README.md`:
```markdown
# Kanvas

A real-time collaborative Kanban board — a portfolio project built to demonstrate clean architecture, tested code, and a real deployment pipeline.

- **Backend:** Go (chi, sqlc + pgx, JWT auth, WebSockets) — see [`backend/README.md`](backend/README.md)
- **Frontend:** React + TypeScript (coming in a later phase)

## Status

Phase 1 (backend foundation + authentication) complete. See [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for implementation plans by phase.
```

- [ ] **Step 6: Commit the packaging and docs changes**

```bash
git add backend/Dockerfile docker-compose.yml backend/README.md README.md
git commit -m "feat(backend): add Dockerfile, compose backend service, and project READMEs"
```

- [ ] **Step 7: Create the GitHub repository and push — ask the user first**

Before running anything in this step, confirm with the user:
- Repository visibility: public or private?
- Confirm the repository name (`kanvas`, under `MatheusCavalari`).

Once confirmed, run (from repo root):
```bash
gh repo create MatheusCavalari/kanvas --source=. --remote=origin --<public-or-private> --push
```
(Replace `--<public-or-private>` with `--public` or `--private` per the user's answer.)

Expected: the command prints the new repository URL, and `git remote -v` shows `origin` pointing at `github.com/MatheusCavalari/kanvas`. Confirm on GitHub that the commit history and files are present.

---

## Definition of Done

- `make test` and `make test-integration` both pass locally.
- `docker compose up -d --build` brings up Postgres + backend, and the full register → me → refresh → logout flow works via `curl`.
- GitHub Actions is green on the pushed branch.
- The repository is live on `github.com/MatheusCavalari/kanvas` with the full commit history from this plan.
