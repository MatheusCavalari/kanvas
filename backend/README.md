# Kanvas backend

Go REST API for Kanvas — Clean/Hexagonal architecture, chi router, sqlc + pgx, JWT auth.

## Prerequisites

- Go 1.23+
- Docker (for local Postgres and for integration tests via testcontainers)

## Local development

### Option A: full stack via Docker Compose

    cp backend/.env.example backend/.env   # optional, compose sets its own env
    docker compose up -d --build           # from the repo root

The API container runs pending SQL migrations automatically on startup
(see `runMigrations` in `cmd/api/main.go`), so a fresh `git clone && docker
compose up -d --build` works with no manual migration step.

### Option B: run the Go binary on the host against Dockerized Postgres

    cp .env.example .env
    docker compose up -d postgres   # from the repo root
    make migrate-up                 # required here — main.go only auto-migrates itself
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
