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
