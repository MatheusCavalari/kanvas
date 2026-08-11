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

## Boards & members

All `/boards` routes require `Authorization: Bearer <access_token>`.

    POST   /boards                        create a board (you become its owner)
    GET    /boards                        list boards you're a member of
    GET    /boards/{boardID}               get a board (must be a member)
    PATCH  /boards/{boardID}               rename a board (must be a member)
    DELETE /boards/{boardID}               delete a board (owner only)
    GET    /boards/{boardID}/members       list members (must be a member)
    POST   /boards/{boardID}/members       invite a member by email (owner only)
    DELETE /boards/{boardID}/members/{userID}  remove a member (owner only)

Inviting a member requires that person to already have a Kanvas account — there's no email-invite flow for non-users yet.

## Columns & cards

All routes below require `Authorization: Bearer <access_token>` and board membership.

    GET    /boards/{boardID}/columns                list columns with their cards, ordered by position
    POST   /boards/{boardID}/columns                 create a column (appended to the end)
    PATCH  /boards/{boardID}/columns/reorder          reorder columns — body: {"column_ids": [...]} (full new order)
    PATCH  /boards/{boardID}/columns/{columnID}       rename a column
    DELETE /boards/{boardID}/columns/{columnID}       delete a column (and its cards)

    POST   /cards                                    create a card — body: {"column_id", "title", "description"?, "assignee_id"?, "due_date"?}
    PATCH  /cards/{cardID}                            update a card's title/description/assignee/due date
    DELETE /cards/{cardID}                            delete a card
    PATCH  /cards/{cardID}/move                       move a card — body: {"column_id", "position"} (works within or across columns)

Card `assignee_id`, if set, must be a registered user's ID (not necessarily a board member — that's not validated in this phase).

## Realtime (WebSocket)

    GET /boards/{boardID}/ws?token=<access_token>

Unlike every other endpoint, the access token is a query parameter, not an `Authorization` header — browsers can't set custom headers on a WebSocket handshake. The connection is authenticated and board-membership-checked before the upgrade; a missing/invalid token gets `401`, a valid token for a non-member gets `403`.

Once connected, the client receives one JSON message per board event, no polling needed:

    {"type": "card.created", "board_id": "...", "data": { ...card fields, same shape as the REST response... }}

Event types: `column.created`, `column.updated`, `column.deleted`, `column.reordered`, `card.created`, `card.updated`, `card.deleted`, `card.moved`. A `*.deleted` event's `data` is just `{"id": "...", ...parent_id}` (the resource is gone); treat it as a signal to refetch that board's columns rather than expecting a separate reorder event for any cleanup renumbering that happened alongside the delete.

The hub is in-process and in-memory: it does not survive a restart and does not work across multiple backend instances — fine for this project's single-instance deployment target, not something to build a multi-instance production system on without swapping in a real pub/sub backend first.

## Tests

    make test              # unit tests (fast, no Docker required)
    make test-integration  # integration + e2e tests (spins up Postgres via testcontainers)

## Project layout

    cmd/api/          entry point
    internal/auth/    auth domain, service, repository, HTTP handlers
    internal/board/   board domain, service, repository, HTTP handlers
    internal/card/    column/card domain, service, repository, HTTP handlers
    internal/realtime/ WebSocket hub, event publisher, realtime HTTP handler
    internal/platform/ shared infra: config, db, jwt, middleware, http router
    db/migrations/    golang-migrate SQL migrations
    db/queries/       sqlc source queries
