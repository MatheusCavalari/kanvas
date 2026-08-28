# Kanvas

**A real-time collaborative Kanban board — built end-to-end (backend, frontend, tests, CI/CD, and production deploy) as a portfolio project.**

[![backend-ci](https://github.com/MatheusCavalari/kanvas/actions/workflows/backend-ci.yml/badge.svg)](https://github.com/MatheusCavalari/kanvas/actions/workflows/backend-ci.yml)
[![frontend-ci](https://github.com/MatheusCavalari/kanvas/actions/workflows/frontend-ci.yml/badge.svg)](https://github.com/MatheusCavalari/kanvas/actions/workflows/frontend-ci.yml)
[![e2e-ci](https://github.com/MatheusCavalari/kanvas/actions/workflows/e2e-ci.yml/badge.svg)](https://github.com/MatheusCavalari/kanvas/actions/workflows/e2e-ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**🔗 Live demo:** [kanvas-frontend-xmez.onrender.com](https://kanvas-frontend-xmez.onrender.com)
*(free-tier backend — the first request after a period of inactivity can take a few seconds to wake up)*

---

## What it is

Kanvas is a Trello/Jira-style Kanban board: boards, columns, and cards, with
members you can invite and drag-and-drop reordering — kept in sync in
**real time** across every browser tab connected to the same board, over a
WebSocket. It's a from-scratch, full-stack build meant to show engineering
practices end-to-end rather than to compete on features:

- **Clean/Hexagonal architecture** on the backend, domain logic tested against
  fakes, no ORM
- **Deep test coverage**: Go unit + Postgres integration tests, frontend
  component tests, and Playwright end-to-end tests exercising the real app
  through a browser
- **Live collaboration** via a WebSocket hub that pushes board mutations to
  every connected client, with automatic reconnect/resync
- **A real CI/CD pipeline**: lint + test + build on every push, end-to-end
  tests against a Dockerized stack, and automatic production deploys

## Screenshots

| | |
|---|---|
| ![Login](docs/screenshots/login.png) **Auth** | ![Board list](docs/screenshots/board-list.png) **Board list** |
| ![Kanban board](docs/screenshots/board-kanban.png) **Kanban view** | ![Card detail](docs/screenshots/card-detail.png) **Card detail** |
| ![Members panel](docs/screenshots/members-panel.png) **Members panel** | |

*(Screenshots are from the live demo above, with sample data.)*

## Features

- **Auth**: register / login / logout, JWT access token + httpOnly refresh
  cookie, transparent silent refresh on expiry
- **Boards**: create, rename, delete; owner and member roles
- **Members**: invite by email, remove, role-gated actions (only the owner
  can invite/remove/delete the board)
- **Columns & cards**: create, rename, delete, edit description; drag-and-drop
  to reorder columns and move cards within or across columns, with the new
  order persisted
- **Labels**: create/edit/delete per board, attach or detach from cards,
  synced live to every open tab
- **Comments**: threaded per-card comments with cursor-based pagination,
  synced live to every open tab
- **Full-text search**: search boards/cards by title and description using
  PostgreSQL `tsvector`
- **Presence**: see who else is viewing a board in real time, tracked via
  WebSocket heartbeats
- **Activity log**: an audit trail of board/card/label/comment mutations,
  viewable per board
- **Webhooks**: register a URL per board to receive HMAC-SHA256-signed
  event payloads, with automatic retry on delivery failure
- **Realtime sync**: every mutation (by you or a teammate) shows up live in
  every open tab on that board, via WebSocket — no polling, no manual refresh
- **Resilience**: the client auto-reconnects and resyncs its state if the
  WebSocket drops; the server shuts down gracefully and exposes health/readiness
  checks
- **Rate limiting**: Redis-backed sliding-window limiter protecting the API
  from abuse
- **Caching**: Redis-backed read caching with automatic invalidation on writes
- **Observability**: structured (`slog`) logs with request correlation IDs,
  and Prometheus metrics at `/metrics`

## Tech stack

| | |
|---|---|
| **Backend** | Go 1.23 · [chi](https://github.com/go-chi/chi) router · [sqlc](https://sqlc.dev/) + [pgx](https://github.com/jackc/pgx) (no ORM) · JWT auth · [coder/websocket](https://github.com/coder/websocket) · PostgreSQL · Redis 7 (cache, rate limiting, background job queue) · [Prometheus](https://prometheus.io/) (`client_golang`) · `log/slog` structured logging |
| **Frontend** | React 19 · TypeScript · Vite · Tailwind CSS v4 · React Router · [TanStack Query](https://tanstack.com/query) · Zustand · [dnd-kit](https://dndkit.com/) |
| **Testing** | Go `testing` + [testcontainers-go](https://testcontainers.com/) (Postgres + Redis integration tests) · Vitest + Testing Library · [Playwright](https://playwright.dev/) (E2E) |
| **Infra** | Docker + Docker Compose (local dev: Postgres, Redis, Prometheus) · GitHub Actions (CI: lint, unit, integration, E2E) · [Render](https://render.com) (production: Docker web service + static site + managed Postgres, via a [Blueprint](https://render.com/docs/blueprint-spec)) |

See [`backend/README.md`](backend/README.md) and
[`frontend/README.md`](frontend/README.md) for the full API reference and
frontend architecture notes.

## Architecture

```
kanvas/
  backend/
    cmd/api/            entry point
    internal/
      auth/              domain, service, repo, HTTP handlers
      board/             boards & members
      card/               columns & cards
      label/               labels, board/card attachment
      comment/             card comments, cursor pagination
      search/              full-text search (tsvector)
      activity/             audit log, EventPublisher decorator
      webhook/              HMAC-signed webhook delivery, EventPublisher decorator
      realtime/            WebSocket hub, presence, event publisher
      platform/
        cache/              Redis read cache + invalidation decorator
        worker/              Redis-backed background job queue
        metrics/             Prometheus collectors
        middleware/           rate limiting, correlation IDs, logging
        httpserver/           graceful shutdown, health/readiness checks
        db, jwt, config, router
    db/
      migrations/         golang-migrate SQL migrations
      queries/             sqlc source queries
  frontend/
    src/
      api/                 HTTP client + per-domain API functions
      features/
        auth/               login/register, session store
        boards/              board list
        board/                kanban view: columns, cards, drag-and-drop, labels,
                              comments, search, presence bar, activity panel
      components/          reusable UI (modal, layout)
  e2e/                    Playwright end-to-end tests
```

Each backend domain (`auth`, `board`, `card`, `label`, `comment`) follows the
same layering: `domain.go` (pure business rules) → `service.go` (use cases,
depends on a repository **interface**) → `repository_postgres.go` (sqlc-backed
implementation) → `handler.go` (chi HTTP handlers). This inversion lets
`service.go` be unit-tested against an in-memory fake repository, with no
database in the loop.

### Event publishing: a decorator chain

Mutating operations (card moved, label attached, comment added, ...) need to
fan out to several side effects — push a WebSocket event, record an activity
log entry, fire any registered webhooks, invalidate cached reads — without
each domain service knowing about all of them. Kanvas solves this with a
shared `EventPublisher` interface and a chain of decorators, each wrapping
the next:

```
service.go  →  ActivityRecorder  →  WebhookDispatcher  →  CacheInvalidator  →  Hub
             (writes audit log)   (fires HMAC webhooks)  (busts Redis cache)  (pushes WS)
```

Each decorator implements `EventPublisher`, does its own work, then forwards
the event to the next one in the chain. A domain service only ever depends on
the `EventPublisher` interface — it has no idea whether an event ends up in
an audit log, a webhook payload, a cache eviction, or a browser tab. Adding a
new side effect (e.g. task 10's job queue) means writing one more decorator
and wiring it into the chain at startup, with no changes to existing domain
code. This mirrors the same "depend on an interface, not a concrete package"
principle used for repositories, and keeps every package free of imports from
its siblings.

On the frontend, [TanStack Query](https://tanstack.com/query) owns all
server state (boards/columns/cards) and its cache is patched directly by
incoming WebSocket events — no refetch needed when a teammate makes a change.

Full design rationale: [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md).
Phase-by-phase implementation plans: [`docs/superpowers/plans/`](docs/superpowers/plans/).

## Running locally

```bash
git clone https://github.com/MatheusCavalari/kanvas.git
cd kanvas
docker compose up -d --build
```

That's it — the API container runs pending migrations automatically. Open
`http://localhost:5173`. The compose stack also brings up **Redis** (caching,
rate limiting, background job queue) and **Prometheus** (scraping the
backend's `/metrics` endpoint at `http://localhost:9090`). See
[`backend/README.md`](backend/README.md) and
[`frontend/README.md`](frontend/README.md) for running each side natively
against Docker's Postgres/Redis instead, and for the full REST/WebSocket API
reference.

## Testing

```bash
# Backend
cd backend && make test              # unit (fast, no Docker)
cd backend && make test-integration  # + Postgres integration tests (testcontainers)

# Frontend
cd frontend && npm test

# End-to-end (spins up the full docker-compose stack)
docker compose up -d --build
cd e2e && npm install && npx playwright test
docker compose down -v
```

All three run in CI on every push — see the badges at the top of this file.

## Deploy

Backend, frontend, and Postgres are defined as a single
[Render Blueprint](https://render.com/docs/blueprint-spec) in
[`render.yaml`](render.yaml): `kanvas-backend` (Docker web service),
`kanvas-frontend` (static site, no cold start), and `kanvas-db` (managed
Postgres) — all on Render's free tier. Once connected to this repo in the
Render dashboard, it redeploys automatically on every push to `master`. CI
gating (lint/test/E2E must pass before merge) is enforced via a GitHub
branch protection rule rather than inside the deploy workflow itself.

See [`docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md`](docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md)
for the full deploy design and the one-time account setup it depends on.

## v2: distributed-systems features

Beyond the MVP, a second phase of work layered in the infrastructure
patterns a real production service needs, each as its own task:

1. **Redis caching** with automatic invalidation on writes, via an
   `EventPublisher` decorator (`CacheInvalidator`)
2. **Graceful shutdown** and Kubernetes-style health checks (`/healthz`,
   `/readyz`)
3. **Structured logging** (`log/slog`) with per-request correlation IDs, and
   **Prometheus metrics** at `/metrics`
4. **Redis-based sliding-window rate limiting** on the API
5. **Labels**: CRUD, attach/detach to cards, realtime events
6. **Card comments**: cursor-based pagination, realtime events
7. **Full-text search** over boards/cards using PostgreSQL `tsvector`
8. **Presence tracking** — who's viewing a board right now — via WebSocket
   heartbeats
9. **Activity/audit log**, recorded via an `EventPublisher` decorator
   (`ActivityRecorder`)
10. **Redis-backed background job queue** with a worker pool pattern
11. **Webhooks**: per-board HTTP callbacks, HMAC-SHA256 request signing, and
    retry-on-failure delivery
12. **Frontend UI** for all of the above: label management, comment threads,
    search, a presence bar, and an activity panel

Engineering patterns this phase set out to demonstrate: composing
cross-cutting concerns as a **decorator chain** behind a single interface
(see [Event publishing](#event-publishing-a-decorator-chain) above) instead
of hardcoding every side effect into each service; keeping caching, rate
limiting, and background work behind small, swappable interfaces so Redis is
an implementation detail rather than a dependency scattered through the
domain layer; and adding observability (structured logs, correlation IDs,
metrics, health checks) as infrastructure that wraps the app rather than
code sprinkled through handlers.

## Known limitations

Documented rather than hidden — things deliberately out of scope for a
portfolio-sized project:

- **Last-write-wins** on concurrent card/column moves — no operational
  transform or conflict resolution.
- No email-invite flow for people who don't already have a Kanvas account.
- The realtime hub is in-process/in-memory: it doesn't survive a restart and
  doesn't fan out across multiple backend instances (fine for this project's
  single-instance deploy; would need a real pub/sub backend, e.g. Redis
  Pub/Sub, to scale out to multiple API instances).
- A user removed from a board while connected to its WebSocket isn't
  proactively disconnected by the server (their next action will fail auth
  checks, but the socket itself stays open until they reload).
- No attachments or file uploads on cards.
- Webhook delivery retries are bounded (no dead-letter queue or manual
  replay UI for permanently-failed deliveries).

## Status

All planned phases are complete and merged: the v1 MVP (backend foundation &
auth, boards & members, columns & cards, realtime WebSocket, the full
frontend, Playwright end-to-end tests, and a live production deploy), plus
the v2 distributed-systems phase described above (caching, observability,
rate limiting, labels, comments, search, presence, activity log, job queue,
webhooks, and their frontend UI).

## License

[MIT](LICENSE) © Matheus Cavalari
