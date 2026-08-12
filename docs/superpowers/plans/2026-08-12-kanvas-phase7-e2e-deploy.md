# Kanvas Phase 7: E2E Testing + Production Deploy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Playwright end-to-end tests (run in CI against a docker-compose stack) and automatic Fly.io production deployment (backend+Postgres, frontend) on merge to `master`.

**Architecture:** A new top-level `e2e/` Playwright project drives the app through a real browser against services started by `docker-compose.yml` (which gains a `frontend` service in this plan). A new `e2e-ci.yml` GitHub Actions workflow runs that suite on every push/PR touching `frontend/`, `backend/`, or `e2e/`. Two `fly.toml` files (one per app) plus a `deploy.yml` workflow push to Fly.io on every push to `master`.

**Tech Stack:** `@playwright/test` (Chromium only), GitHub Actions, Fly.io (`flyctl`), Docker Compose.

## Global Constraints

- Playwright runs **Chromium only** in this plan — no cross-browser matrix. Scope decision to keep CI fast; add other browser projects later if needed.
- Every E2E test creates its own user via the UI's register flow with a unique, timestamp-based email — no shared fixtures, no seed data, no cleanup step (each CI run's containers are thrown away).
- The Phase 7 design's "second tab" realtime scenario is implemented as **two `Page` objects from the same Playwright `BrowserContext`**, not two separate `BrowserContext`s — this is required for the second page to inherit the first's httpOnly refresh-token cookie and land on the same authenticated session (the design doc's "second tab" language already implies same-user, same-browser; two separate `BrowserContext`s would each need their own login, which doesn't match "a second tab").
- Card/column drag interactions in E2E tests MUST use raw `page.mouse.down()` / `page.mouse.move(..., { steps })` / `page.mouse.up()` sequences, never Playwright's `locator.dragTo()` — `dragTo()` dispatches native HTML5 `dragstart`/`dragover`/`drop` DOM events, but this app's drag-and-drop (`@dnd-kit`, Phase 6) listens for pointer events via `PointerSensor`, not native HTML5 DnD. The mouse-move sequence must include at least one intermediate move exceeding 5px of travel, matching `BoardPage.tsx`'s `activationConstraint: { distance: 5 }` — a drag that never exceeds 5px never activates.
- Fly app names are fixed: **`kanvas-backend`** and **`kanvas-frontend`**, primary region **`gru`** (São Paulo). If either name is taken on Fly.io when the user runs `fly apps create`, they substitute the assigned name into both `fly.toml` files and `deploy.yml` before the first deploy — this plan's tasks use the fixed names as final values, not placeholders, on the assumption they're available.
- Deploy gating on CI passing is achieved via **GitHub branch protection on `master`** (a repo-settings change), not via `workflow_run` chaining — `deploy.yml` triggers directly on `push: branches: [master]`. This is a manual, one-time repo configuration step (listed at the end of this plan), not part of any dispatched task; no task in this plan modifies GitHub repo settings.
- Nothing in this plan changes application code's runtime behavior (no new backend endpoints, no new frontend features) except two `data-testid` attributes added to `frontend/src/features/board/Column.tsx` purely to give the E2E drag test stable selectors — everything else is test/CI/deploy configuration.
- `frontend/src/features/board/Column.tsx`, `frontend/src/features/board/CardItem.tsx`, `frontend/src/features/boards/BoardListPage.tsx`, `frontend/src/features/board/BoardPage.tsx`, `frontend/src/features/auth/LoginPage.tsx`, `frontend/src/features/auth/RegisterPage.tsx`, and `frontend/src/components/layout/AppLayout.tsx` already exist from Phases 5-6 with exact label/button text this plan's E2E specs depend on verbatim (e.g. "Entrar", "Criar conta", "Novo board", "Seus boards", "+ Adicionar coluna", "Sair") — do not guess these strings, they are given exactly in each task below.

---

## Task 1: Playwright E2E project — docker-compose frontend service, scaffold, and 4 specs

**Files:**
- Modify: `docker-compose.yml`
- Modify: `frontend/src/features/board/Column.tsx`
- Create: `e2e/package.json`
- Create: `e2e/playwright.config.ts`
- Create: `e2e/tsconfig.json`
- Create: `e2e/tests/helpers.ts`
- Create: `e2e/tests/auth.spec.ts`
- Create: `e2e/tests/create-board.spec.ts`
- Create: `e2e/tests/move-card.spec.ts`
- Create: `e2e/tests/realtime.spec.ts`
- Create: `e2e/.gitignore`

**Interfaces:**
- Produces: a runnable `e2e/` Playwright project (`npm test` inside `e2e/` runs `playwright test`), consumed by Task 2's CI workflow with the exact same commands.

- [ ] **Step 1: Add a `frontend` service to `docker-compose.yml`**

Change `docker-compose.yml` from:

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
      MIGRATIONS_PATH: /app/db/migrations
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  postgres_data:
```

to:

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
      MIGRATIONS_PATH: /app/db/migrations
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy

  frontend:
    build:
      context: ./frontend
      args:
        VITE_API_URL: http://localhost:8080
    ports:
      - "5173:80"
    depends_on:
      - backend

volumes:
  postgres_data:
```

The host port `5173` matches `backend/internal/platform/config/config.go`'s default `CORS_ALLOWED_ORIGIN` (`http://localhost:5173`), so no extra CORS env var is needed on the `backend` service — the existing default already allows this origin.

- [ ] **Step 2: Add `data-testid` attributes to `Column.tsx` for stable E2E selectors**

In `frontend/src/features/board/Column.tsx`, change the root element:

```tsx
    <div ref={setColumnNodeRef} style={columnStyle} className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">
```

to:

```tsx
    <div ref={setColumnNodeRef} style={columnStyle} data-testid="column" className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">
```

and the droppable card-list container:

```tsx
      <div ref={setDroppableRef} className="flex flex-col gap-2">
```

to:

```tsx
      <div ref={setDroppableRef} data-testid="column-cards" className="flex flex-col gap-2">
```

No other line in this file changes.

- [ ] **Step 3: Scaffold the Playwright project**

`e2e/package.json`:

```json
{
  "name": "e2e",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "test": "playwright test"
  },
  "devDependencies": {
    "@playwright/test": "^1.49.0",
    "typescript": "~5.8.3"
  }
}
```

`e2e/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "strict": true,
    "skipLibCheck": true,
    "types": ["@playwright/test"]
  },
  "include": ["tests/**/*.ts"]
}
```

`e2e/.gitignore`:

```
node_modules/
playwright-report/
test-results/
```

`e2e/playwright.config.ts`:

```ts
import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
```

From `e2e/`, run:

```bash
npm install
npx playwright install --with-deps chromium
```

- [ ] **Step 4: Write the shared test helpers**

`e2e/tests/helpers.ts`:

```ts
import type { Locator, Page } from '@playwright/test'

export function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.floor(Math.random() * 100000)}@example.com`
}

/**
 * Registers a fresh user via the UI, landing on the board list. Returns the
 * generated email in case a test needs to log back in with it later.
 */
export async function registerAndLogin(page: Page, namePrefix: string): Promise<string> {
  const email = uniqueEmail(namePrefix)
  await page.goto('/register')
  await page.getByLabel('Nome').fill(`${namePrefix} User`)
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('password123')
  await page.getByRole('button', { name: 'Criar conta' }).click()
  await page.getByRole('heading', { name: 'Seus boards' }).waitFor()
  return email
}

export async function createBoardAndOpen(page: Page, name: string): Promise<void> {
  await page.getByRole('button', { name: 'Novo board' }).click()
  await page.getByLabel('Nome').fill(name)
  await page.getByRole('button', { name: 'Criar' }).click()
  await page.getByRole('link', { name }).click()
}

export async function addColumn(page: Page, title: string): Promise<void> {
  await page.getByRole('button', { name: '+ Adicionar coluna' }).click()
  await page.getByLabel('Título da coluna').fill(title)
  await page.getByRole('button', { name: 'Adicionar', exact: true }).click()
}

export function columnByTitle(page: Page, title: string): Locator {
  return page.getByTestId('column').filter({ hasText: title })
}

export async function addCard(page: Page, columnTitle: string, cardTitle: string): Promise<void> {
  const column = columnByTitle(page, columnTitle)
  await column.getByRole('button', { name: '+ Adicionar card' }).click()
  await page.getByLabel('Título do card').fill(cardTitle)
  await column.getByRole('button', { name: 'Adicionar', exact: true }).click()
}

/**
 * Drags `source` onto `target` using raw pointer events — this app's
 * drag-and-drop (@dnd-kit) listens for pointer events, not native HTML5
 * drag-and-drop, so Locator.dragTo() (which dispatches HTML5 DnD events)
 * does not work here.
 */
export async function dragElement(page: Page, source: Locator, target: Locator): Promise<void> {
  const sourceBox = await source.boundingBox()
  const targetBox = await target.boundingBox()
  if (!sourceBox || !targetBox) {
    throw new Error('drag source or target is not visible')
  }

  const startX = sourceBox.x + sourceBox.width / 2
  const startY = sourceBox.y + sourceBox.height / 2
  const endX = targetBox.x + targetBox.width / 2
  const endY = targetBox.y + targetBox.height / 2

  await page.mouse.move(startX, startY)
  await page.mouse.down()
  // Exceed dnd-kit's PointerSensor activation distance (5px) before the
  // real move, otherwise the drag never activates.
  await page.mouse.move(startX + 10, startY + 10, { steps: 5 })
  await page.mouse.move(endX, endY, { steps: 10 })
  await page.mouse.up()
}
```

- [ ] **Step 5: Write the auth flow spec**

`e2e/tests/auth.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { uniqueEmail } from './helpers'

test('user can register, log out, and log back in', async ({ page }) => {
  const email = uniqueEmail('auth')
  const password = 'password123'

  await page.goto('/register')
  await page.getByLabel('Nome').fill('Ada Lovelace')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Criar conta' }).click()

  await expect(page.getByRole('heading', { name: 'Seus boards' })).toBeVisible()

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login/)

  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill(password)
  await page.getByRole('button', { name: 'Entrar' }).click()

  await expect(page.getByRole('heading', { name: 'Seus boards' })).toBeVisible()
})
```

- [ ] **Step 6: Write the create-board spec**

`e2e/tests/create-board.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { registerAndLogin } from './helpers'

test('user can create a board and open its empty kanban view', async ({ page }) => {
  await registerAndLogin(page, 'create-board')

  await page.getByRole('button', { name: 'Novo board' }).click()
  await page.getByLabel('Nome').fill('Sprint Board')
  await page.getByRole('button', { name: 'Criar' }).click()

  const boardLink = page.getByRole('link', { name: 'Sprint Board' })
  await expect(boardLink).toBeVisible()
  await boardLink.click()

  await expect(page.getByRole('button', { name: '+ Adicionar coluna' })).toBeVisible()
})
```

- [ ] **Step 7: Write the create/move card spec**

`e2e/tests/move-card.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { registerAndLogin, createBoardAndOpen, addColumn, addCard, columnByTitle, dragElement } from './helpers'

test('user can drag a card to a different column and it persists after reload', async ({ page }) => {
  await registerAndLogin(page, 'move-card')
  await createBoardAndOpen(page, 'Kanban Board')

  await addColumn(page, 'To Do')
  await addColumn(page, 'Done')
  await addCard(page, 'To Do', 'Ship it')

  const card = page.getByRole('button', { name: 'Ship it' })
  const doneColumnCards = columnByTitle(page, 'Done').getByTestId('column-cards')

  await dragElement(page, card, doneColumnCards)

  await expect(doneColumnCards.getByRole('button', { name: 'Ship it' })).toBeVisible()

  await page.reload()

  await expect(columnByTitle(page, 'Done').getByTestId('column-cards').getByRole('button', { name: 'Ship it' })).toBeVisible()
  await expect(columnByTitle(page, 'To Do').getByRole('button', { name: 'Ship it' })).not.toBeVisible()
})
```

- [ ] **Step 8: Write the realtime cross-tab spec**

`e2e/tests/realtime.spec.ts`:

```ts
import { test, expect } from '@playwright/test'
import { registerAndLogin, createBoardAndOpen, addColumn, addCard, columnByTitle } from './helpers'

test('a card created in one tab appears in another tab viewing the same board', async ({ context }) => {
  const ownerPage = await context.newPage()
  await registerAndLogin(ownerPage, 'realtime')
  await createBoardAndOpen(ownerPage, 'Realtime Board')
  await addColumn(ownerPage, 'Inbox')

  const boardUrl = ownerPage.url()

  // A second tab in the SAME browser context: it shares the httpOnly
  // refresh-token cookie, so it restores the same session on load — this
  // is what makes it "a second tab", not a second user.
  const viewerPage = await context.newPage()
  await viewerPage.goto(boardUrl)
  await expect(viewerPage.getByRole('button', { name: '+ Adicionar coluna' })).toBeVisible()

  await addCard(ownerPage, 'Inbox', 'Realtime card')

  await expect
    .poll(
      async () => columnByTitle(viewerPage, 'Inbox').getByRole('button', { name: 'Realtime card' }).count(),
      { timeout: 10000 },
    )
    .toBeGreaterThan(0)
})
```

- [ ] **Step 9: Verify all four specs pass against the local docker-compose stack**

From the repository root:

```bash
docker compose up -d --build
```

Poll until ready (both must respond):

```bash
curl --retry 20 --retry-delay 2 --retry-connrefused http://localhost:8080/healthz
curl --retry 20 --retry-delay 2 --retry-connrefused http://localhost:5173/
```

Then, from `e2e/`:

```bash
npx playwright test
```

Expected: all 4 specs pass. If `move-card.spec.ts` or `realtime.spec.ts` fail on the drag step, open `playwright-report/index.html` (generated on failure) and check whether the drag's intermediate mouse-move steps actually cleared 5px of travel before the final move — a report screenshot showing the card still in its origin column with no drag-overlay artifact means the pointer-down never registered as a drag.

Tear down:

```bash
docker compose down -v
```

- [ ] **Step 10: Commit**

```bash
git add docker-compose.yml frontend/src/features/board/Column.tsx e2e/
git commit -m "test(e2e): add Playwright E2E suite (auth, boards, drag, realtime)"
```

---

## Task 2: CI workflow for E2E

**Files:**
- Create: `.github/workflows/e2e-ci.yml`

**Interfaces:**
- Consumes: `e2e/` (Task 1), `docker-compose.yml` (Task 1).

- [ ] **Step 1: Write the workflow**

`.github/workflows/e2e-ci.yml`:

```yaml
name: e2e-ci

on:
  push:
    paths:
      - "frontend/**"
      - "backend/**"
      - "e2e/**"
      - "docker-compose.yml"
      - ".github/workflows/e2e-ci.yml"
  pull_request:
    paths:
      - "frontend/**"
      - "backend/**"
      - "e2e/**"
      - "docker-compose.yml"
      - ".github/workflows/e2e-ci.yml"

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Start the stack
        run: docker compose up -d --build

      - name: Wait for backend
        run: |
          for i in $(seq 1 30); do
            if curl -sf http://localhost:8080/healthz > /dev/null; then
              echo "backend is up"
              exit 0
            fi
            sleep 2
          done
          echo "backend did not become healthy in time"
          docker compose logs backend
          exit 1

      - name: Wait for frontend
        run: |
          for i in $(seq 1 30); do
            if curl -sf http://localhost:5173/ > /dev/null; then
              echo "frontend is up"
              exit 0
            fi
            sleep 2
          done
          echo "frontend did not become healthy in time"
          docker compose logs frontend
          exit 1

      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: e2e/package-lock.json

      - name: Install E2E dependencies
        working-directory: e2e
        run: npm install

      - name: Install Playwright browsers
        working-directory: e2e
        run: npx playwright install --with-deps chromium

      - name: Run Playwright tests
        working-directory: e2e
        run: npx playwright test

      - name: Upload Playwright report
        if: failure()
        uses: actions/upload-artifact@v4
        with:
          name: playwright-report
          path: e2e/playwright-report/
          retention-days: 7

      - name: Tear down the stack
        if: always()
        run: docker compose down -v
```

`npm install` (not `npm ci`) is used here because Task 1 does not commit a `package-lock.json` for `e2e/` as part of its steps — running `npm install` locally in Task 1's Step 3 generates one, which gets committed in Task 1's Step 10 (it's inside `e2e/`, not excluded by `e2e/.gitignore`). Once that lockfile exists, this workflow's `cache-dependency-path` resolves correctly on every run after the first.

- [ ] **Step 2: Verify the workflow is syntactically valid**

```bash
cd "$(git rev-parse --show-toplevel)"
python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/e2e-ci.yml'))" 2>&1 || (command -v yq >/dev/null && yq eval '.' .github/workflows/e2e-ci.yml > /dev/null)
```

(Use whichever YAML validator is available in the environment — the goal is just to catch indentation errors before pushing; GitHub Actions itself is the real validator once this reaches a PR.)

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/e2e-ci.yml
git commit -m "ci: run Playwright E2E suite against docker-compose stack"
```

---

## Task 3: Backend Fly.io app configuration

**Files:**
- Create: `backend/fly.toml`

**Interfaces:**
- Produces: `backend/fly.toml`, consumed by Task 5's `deploy.yml`.

- [ ] **Step 1: Write `backend/fly.toml`**

```toml
app = "kanvas-backend"
primary_region = "gru"

[build]
  dockerfile = "Dockerfile"

[deploy]
  strategy = "rolling"

[env]
  PORT = "8080"
  MIGRATIONS_PATH = "/app/db/migrations"

[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = false
  auto_start_machines = true
  min_machines_running = 1

[[http_service.checks]]
  grace_period = "10s"
  interval = "15s"
  method = "GET"
  timeout = "5s"
  path = "/healthz"
```

`DATABASE_URL`, `JWT_SECRET`, and `CORS_ALLOWED_ORIGIN` are deliberately absent from `[env]` — they're set as Fly secrets (`fly secrets set`) by the user during the manual setup steps at the end of this plan, never committed to this file.

- [ ] **Step 2: Validate the TOML syntax**

```bash
cd "$(git rev-parse --show-toplevel)/backend"
python3 -c "import tomllib; tomllib.load(open('fly.toml', 'rb'))"
```

Expected: no output (no exception raised) — confirms the file parses as valid TOML. This does not validate Fly-specific semantics (that requires `flyctl`, which the user runs once they've completed the manual account setup).

- [ ] **Step 3: Commit**

```bash
git add backend/fly.toml
git commit -m "chore(backend): add Fly.io app configuration"
```

---

## Task 4: Frontend Fly.io app configuration

**Files:**
- Create: `frontend/fly.toml`

**Interfaces:**
- Produces: `frontend/fly.toml`, consumed by Task 5's `deploy.yml`.

- [ ] **Step 1: Write `frontend/fly.toml`**

```toml
app = "kanvas-frontend"
primary_region = "gru"

[build]
  dockerfile = "Dockerfile"

[deploy]
  strategy = "rolling"

[http_service]
  internal_port = 80
  force_https = true
  auto_stop_machines = false
  auto_start_machines = true
  min_machines_running = 1

[[http_service.checks]]
  grace_period = "10s"
  interval = "15s"
  method = "GET"
  timeout = "5s"
  path = "/"
```

`frontend/Dockerfile` already declares `ARG VITE_API_URL` (from Phase 5) — this file doesn't need to reference it; the build-time value is supplied via `flyctl deploy --build-arg` in Task 5's workflow, not via `fly.toml`.

- [ ] **Step 2: Validate the TOML syntax**

```bash
cd "$(git rev-parse --show-toplevel)/frontend"
python3 -c "import tomllib; tomllib.load(open('fly.toml', 'rb'))"
```

Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add frontend/fly.toml
git commit -m "chore(frontend): add Fly.io app configuration"
```

---

## Task 5: Deploy workflow and README update

**Files:**
- Create: `.github/workflows/deploy.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: `backend/fly.toml` (Task 3), `frontend/fly.toml` (Task 4).

- [ ] **Step 1: Write the deploy workflow**

`.github/workflows/deploy.yml`:

```yaml
name: deploy

on:
  push:
    branches: [master]

jobs:
  deploy-backend:
    runs-on: ubuntu-latest
    concurrency: deploy-backend
    steps:
      - uses: actions/checkout@v4
      - uses: superfly/flyctl-actions/setup-flyctl@master
      - name: Deploy backend
        working-directory: backend
        run: flyctl deploy --remote-only
        env:
          FLY_API_TOKEN: ${{ secrets.FLY_API_TOKEN }}

  deploy-frontend:
    runs-on: ubuntu-latest
    concurrency: deploy-frontend
    steps:
      - uses: actions/checkout@v4
      - uses: superfly/flyctl-actions/setup-flyctl@master
      - name: Deploy frontend
        working-directory: frontend
        run: flyctl deploy --remote-only --build-arg VITE_API_URL=https://kanvas-backend.fly.dev
        env:
          FLY_API_TOKEN: ${{ secrets.FLY_API_TOKEN }}
```

`--remote-only` builds each image on Fly's remote builders rather than requiring Docker on the GitHub Actions runner. `working-directory` combined with each app having its own `fly.toml` (Tasks 3-4) means `flyctl` auto-discovers the right config without an explicit `--config` flag or `--app` flag (the `app = "..."` line inside each `fly.toml` supplies it).

This workflow triggers on every push to `master`, which — per this plan's Global Constraints — is gated on CI passing via GitHub branch protection (a manual step below), not via workflow chaining inside this file.

- [ ] **Step 2: Update the root README's status section**

In `README.md`, find the `## Status` section (currently ending with the Phase 5 description) and replace its content with:

```markdown
## Status

Phases 1-7 complete: backend foundation, authentication, boards &
members, columns & cards, realtime WebSocket, a full frontend (auth,
board list, kanban view with drag-and-drop, members panel, realtime
sync), Playwright end-to-end tests, and automatic deployment to Fly.io
on every merge to `master`. See
[`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md)
for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/)
for implementation plans by phase.

## Deploy

Backend and frontend each deploy to their own Fly.io app
(`kanvas-backend`, `kanvas-frontend`) automatically on every push to
`master`, via [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml).
See [`docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md`](docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md)
for the one-time Fly.io account setup this depends on.
```

Read the current `README.md` first to confirm the exact heading text and surrounding structure before editing — match its existing Markdown conventions (heading levels, link style) rather than guessing.

- [ ] **Step 3: Validate the new workflow's YAML syntax**

```bash
cd "$(git rev-parse --show-toplevel)"
python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/deploy.yml'))"
```

Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/deploy.yml README.md
git commit -m "ci: add Fly.io deploy workflow, update README status"
```

---

## Manual steps (the user performs these; not part of any dispatched task)

1. Create a Fly.io account, install `flyctl`, `fly auth login`.
2. `fly apps create kanvas-backend` and `fly apps create kanvas-frontend` (reserve the names; if either is taken, substitute the assigned name into the corresponding `fly.toml` and into `deploy.yml`'s `VITE_API_URL` build-arg before continuing).
3. `fly postgres create`, then `fly postgres attach --app kanvas-backend` (sets `DATABASE_URL` as a Fly secret automatically).
4. `fly secrets set JWT_SECRET=<a-real-secret> CORS_ALLOWED_ORIGIN=https://kanvas-frontend.fly.dev --app kanvas-backend`.
5. `fly tokens create deploy`, then add the result as the `FLY_API_TOKEN` secret in the GitHub repo's Settings → Secrets and variables → Actions.
6. In GitHub repo Settings → Branches, add a branch protection rule on `master` requiring the `backend-ci`, `frontend-ci`, and `e2e-ci` status checks to pass before merging — this is what makes `deploy.yml`'s "gated on CI passing" requirement real, since `deploy.yml` itself triggers unconditionally on push to `master`.
7. Merge this plan's PR to `master` (triggers the first real deploy), or run `flyctl deploy --remote-only` manually from each app's directory once to seed the first release.
