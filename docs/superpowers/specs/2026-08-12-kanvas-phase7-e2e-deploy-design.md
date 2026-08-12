# Kanvas Phase 7: E2E Testing + Production Deploy

> **Update (2026-08-12):** the deploy target described below (Fly.io) was
> replaced with [Render.com](https://render.com) after discovering Fly.io
> now requires a credit card even on its lowest usage tier — free-tier
> machines are stopped after 5 minutes without one. Render's free tier
> (web services + static sites + a small Postgres instance) requires no
> card. The E2E testing sections below are unaffected and still accurate.
> Everything under "Fly.io Deploy" and "Manual steps" describes the
> superseded design; see [`render.yaml`](../../../render.yaml) and the
> README's Deploy section for what's actually running.

## Goal

Close out the two DevOps pieces the original design called for but no
phase has built yet: end-to-end browser tests (Playwright) run in CI
against a docker-compose stack, and automatic production deployment to
Fly.io (backend + Postgres, frontend) on merge to `master`.

## Context

Phases 1-6 are complete and merged: full backend (auth, boards/members,
columns/cards, realtime WebSocket, hardened WS Origin check), and a full
frontend (auth, board list, kanban view with drag-and-drop, members panel,
realtime sync). Docker (`docker-compose.yml`, `backend/Dockerfile`,
`frontend/Dockerfile`) and per-directory CI (`backend-ci.yml`,
`frontend-ci.yml` — lint + unit/integration tests) already exist and are
unaffected by this phase.

The original master design
(`docs/superpowers/specs/2026-08-10-kanvas-design.md`, sections 8-9)
specifies:
- Playwright E2E covering login, create board, create/move card, and a
  second tab receiving a realtime update over WebSocket.
- A CI job running E2E against the docker-compose stack.
- Two Fly.io apps (backend+Postgres, frontend-as-static-via-Nginx) with
  automatic `flyctl deploy` on merge to the main branch, gated on CI
  passing.

The user does not yet have a Fly.io account. Account creation, `flyctl`
login, `fly launch`/`fly postgres create` (which need an authenticated
session), and setting the `FLY_API_TOKEN` GitHub secret are manual steps
the user performs themselves — this plan prepares every config file and
workflow so those manual steps are the only gap between "plan merged" and
"live in production."

## Playwright E2E

- New top-level directory `e2e/`, with its own `package.json` and
  `playwright.config.ts` (a separate Node project from `frontend/`, since
  it tests the running system rather than compiling into it).
- `baseURL` in the Playwright config points at the frontend service as
  exposed by `docker-compose.yml`.
- Each test creates its own user via the UI's register flow, with a
  unique email (e.g. `test-${Date.now()}-${randomSuffix}@example.com`) —
  no shared fixtures or seed data, no cross-test collisions, no cleanup
  step needed since each CI run is a fresh set of throwaway containers.
- Four specs, matching the original design's four flows:
  1. **Auth flow**: register → land on board list → logout → login again.
  2. **Create board**: login, create a board via the UI, assert it
     appears in the list and the kanban view loads with zero columns.
  3. **Create/move card**: create a board, create two columns, create a
     card, drag it from one column to the other, reload the page, assert
     the card is still in the second column (proves the mutation
     persisted, not just the optimistic-looking drag animation).
  4. **Realtime across tabs**: two `BrowserContext`s (simulating two
     tabs/users) both viewing the same board; one creates a card, the
     other's view updates without a manual reload, asserted via
     `expect.poll` (never a fixed `sleep`) against the second context's
     DOM.
- Test 3's drag step is a real Playwright pointer drag (`mouse.down` /
  `mouse.move` / `mouse.up` with intermediate move steps) — Playwright
  drives real browser input events, so this is not subject to the jsdom
  limitation that blocked drag automation in the Phase 6 unit tests.

## CI: `e2e-ci.yml`

- New workflow file `.github/workflows/e2e-ci.yml`, triggered on `push`
  and `pull_request` with `paths: ["frontend/**", "backend/**",
  "e2e/**", ".github/workflows/e2e-ci.yml"]` — same path-filter
  convention as the existing `backend-ci.yml`/`frontend-ci.yml`.
- Steps: checkout → `docker compose up -d --build` → poll the backend's
  `/healthz` and the frontend's root URL until both respond (a short
  bash loop with a timeout, failing the job if the stack never becomes
  healthy) → install Playwright + browsers in `e2e/` → `npx playwright
  test` → upload the Playwright HTML report as a build artifact on
  failure (`if: failure()`) for debugging → `docker compose down -v` in
  an `if: always()` step so containers are cleaned up whether the tests
  passed, failed, or the job was cancelled.

## Fly.io Deploy

- **Two Fly apps**: one for the backend (Go binary, connects to a Fly
  Postgres cluster), one for the frontend (the existing Nginx-served
  static build from `frontend/Dockerfile`).
- **App names**: `kanvas-backend` and `kanvas-frontend` — their public
  URLs will be `https://kanvas-backend.fly.dev` and
  `https://kanvas-frontend.fly.dev` unless the user's chosen names are
  already taken on Fly.io, in which case the user substitutes the actual
  assigned names into both `fly.toml` files and the deploy workflow's
  `--app` flags before the first deploy (Fly's `fly launch` reports the
  final name/URL if the requested one is unavailable).
- **`backend/fly.toml`**: `app = "kanvas-backend"`, `[build]` pointing at
  `backend/Dockerfile`, `[deploy] strategy = "rolling"` (explicit, not
  relying on the default, so a bad release never takes over traffic from
  a healthy one), an HTTP health check against `/healthz`, and `[env]`
  entries for non-secret config (`PORT`, `MIGRATIONS_PATH`); secrets
  (`DATABASE_URL`, `JWT_SECRET`, `CORS_ALLOWED_ORIGIN`) are set via `fly
  secrets set`, never committed to `fly.toml`. `CORS_ALLOWED_ORIGIN` is
  set to `https://kanvas-frontend.fly.dev`.
- **`frontend/fly.toml`**: `app = "kanvas-frontend"`, `[build]` pointing
  at `frontend/Dockerfile`, `[deploy] strategy = "rolling"`, health check
  against `/`. The frontend's `VITE_API_URL` is a Vite build-time env
  var, not a runtime one — it is passed as a `--build-arg
  VITE_API_URL=https://kanvas-backend.fly.dev` to `flyctl deploy` (baked
  into the Dockerfile's build stage via an `ARG`/`ENV`).
- **`.github/workflows/deploy.yml`**: triggered on `push` to `master`
  only, unconditionally — GitHub Actions doesn't let a `push`-triggered
  workflow `needs:` jobs from separately-triggered workflows in the same
  run, so `deploy.yml` itself has no built-in gate on CI passing. The
  "gated on CI passing" requirement is instead satisfied by a branch
  protection rule on `master` (a manual, one-time repo configuration
  step — see "Manual steps" below) requiring the `backend-ci`,
  `frontend-ci`, and `e2e-ci` status checks to pass before a PR can be
  merged. By the time a commit lands on `master` and `deploy.yml` fires,
  those checks have therefore already run and passed on that code. Two
  jobs, `deploy-backend` and `deploy-frontend`, each runs `flyctl deploy
  --app <app-name>` using the `superfly/flyctl-actions/setup-flyctl`
  action (pinned to a release tag, not a mutable branch ref) and
  `FLY_API_TOKEN` from GitHub secrets.
- **Backend↔frontend origin wiring**: the same Origin-check path
  hardened in Phase 6 applies unchanged in production — `kanvas-backend`
  only accepts WebSocket/CORS requests from `kanvas-frontend`'s origin,
  purely via the configuration above, no code changes needed here.

## Manual steps (the user performs these; not part of any dispatched task)

1. Create a Fly.io account, install `flyctl`, `fly auth login`.
2. `fly apps create kanvas-backend` and `fly apps create kanvas-frontend`
   (without deploying yet — just to reserve the names; if either is
   taken, substitute the assigned name into both `fly.toml` files and the
   deploy workflow before continuing).
3. `fly postgres create` for the database, then `fly postgres attach --app kanvas-backend`
   (this sets `DATABASE_URL` as a Fly secret on the backend app
   automatically).
4. `fly secrets set JWT_SECRET=... CORS_ALLOWED_ORIGIN=https://kanvas-frontend.fly.dev --app kanvas-backend`.
5. Add `FLY_API_TOKEN` (from `fly tokens create deploy`) as a GitHub
   Actions repository secret.
6. Add a branch protection rule on `master` (GitHub repo Settings →
   Branches) requiring the `backend-ci`, `frontend-ci`, and `e2e-ci`
   status checks to pass before merging — this is what makes
   `deploy.yml`'s "gated on CI passing" design actually true, since
   `deploy.yml` itself triggers unconditionally on push to `master` with
   no built-in gating. Note: because each of those three workflows only
   runs when its path filters match (see each workflow's `on.push.paths`),
   a PR that touches none of those paths (e.g. a docs-only change) will
   leave the required checks permanently "pending" and unable to merge —
   if that's ever a problem in practice, either broaden a workflow's path
   filters or use a required-check bypass for docs-only PRs; not
   addressed further here since it hasn't come up yet.
7. Trigger the first deploy manually once (`flyctl deploy --app
   kanvas-backend` / `--app kanvas-frontend`), or merge this phase's PR
   to `master` (which triggers `deploy.yml` automatically thereafter).

## Errors and edge cases

- **E2E flakiness**: the realtime cross-tab test uses `expect.poll`
  against the second `BrowserContext`'s rendered DOM state, not a fixed
  `sleep` — avoids both flaky failures under CI load and unnecessarily
  slow tests.
- **CI stack never becomes healthy**: the health-check polling loop in
  `e2e-ci.yml` has a timeout (e.g. 60s) and fails the job with the
  `docker compose logs` output captured, rather than hanging until the
  job-level timeout.
- **Deploy failure**: Fly's rolling strategy keeps the previous release
  serving traffic until the new one passes its health check — no custom
  rollback logic in this v1. A migration failure on boot causes
  `log.Fatalf` (existing behavior, unchanged), which fails the health
  check and keeps the prior release live.
- **Cleanup on E2E job failure/cancellation**: `docker compose down -v`
  runs in an `if: always()` step so a failed or cancelled run never
  leaves containers/volumes behind on the runner.

## Out of scope

- The original design's edge case "a user removed from a board while
  connected via that board's WebSocket should have the connection closed
  by the server" is **not implemented in any phase to date** and is not
  part of this phase either — it's a backend behavioral feature, not
  testing/deploy infrastructure. Flagged here as a known gap for a future
  phase.
- Custom deploy rollback tooling beyond Fly's built-in rolling-release
  behavior.
- E2E coverage beyond the four flows listed above (no labels/search/etc.
  — those features don't exist yet either).
- Staging environment — this phase deploys straight to a single
  production environment, matching the original design's scope.
