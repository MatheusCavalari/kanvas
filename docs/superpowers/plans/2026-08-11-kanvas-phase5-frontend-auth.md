# Kanvas Phase 5 — Frontend Setup + Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the `frontend/` React + TypeScript project and implement a full authentication flow (register, login, silent session restore, automatic access-token refresh, logout) against the real Kanvas backend, ending at an authenticated placeholder page.

**Architecture:** Vite + React + TypeScript frontend in `frontend/` at the repo root, alongside the existing `backend/`. Zustand holds session state (user + in-memory access token) with no persistence to `localStorage`. A thin `fetch` wrapper (`api/client.ts`) centralizes auth-header injection and a one-shot 401 → refresh → retry cycle. React Router splits public (`/login`, `/register`) from protected (`/`) routes. Because the frontend runs on a different origin (Vite dev server) than the backend, this phase also adds CORS support to the backend — without it, the browser blocks the credentialed requests the refresh-cookie flow depends on.

**Tech Stack:** Vite, React 18, TypeScript, Tailwind CSS v4, React Router v6, Zustand, Vitest + Testing Library, ESLint. Backend: `github.com/go-chi/cors`.

## Global Constraints

- The backend's access token is never written to `localStorage` or `sessionStorage` — in-memory only, per the design's XSS-reduction rationale (spec §3, referencing the original design's §6). Implementation note: the token's raw value lives in `api/client.ts`'s module scope (Task 3) rather than in the Zustand store itself, since only the HTTP client needs it to build headers; the store (Task 4) exposes `user`/`status`, which is all any component needs. This still satisfies the spec's actual intent — in-memory, never persisted — while avoiding threading the raw token through component re-renders.
- The refresh token is an `httpOnly` cookie already managed entirely by the backend (`Path: /auth`) — the frontend never reads or writes it directly, only relies on the browser sending it automatically to `/auth/refresh` and `/auth/logout`.
- Every authenticated `fetch` goes through `api/client.ts` — no ad-hoc `fetch` calls elsewhere in the app, so the 401-refresh-retry behavior and auth header injection stay centralized.
- Error responses from the backend follow `{"error":{"code":"...","message":"..."}}` (see `backend/internal/card/handler.go`'s `writeError` / `backend/internal/auth/handler.go`'s `writeError` for the authoritative shape) — the frontend's error handling must read `body.error.code` / `body.error.message`, not assume plain text.
- All `fetch` calls to the backend must set `credentials: "include"` so the `httpOnly` refresh cookie is sent/received cross-origin.
- Component/hook tests use Vitest + Testing Library with mocked `fetch` (via `vi.fn()`/`vi.stubGlobal`) — no real network calls in unit tests, no Playwright in this phase.
- New frontend code follows the file layout in the design spec (`docs/superpowers/specs/2026-08-11-kanvas-phase5-frontend-auth-design.md` §2) unless a task explicitly says otherwise.

---

### Task 1: Backend — add CORS support for the frontend origin

**Why this is in a "frontend" phase:** the design assumed CORS already worked; it doesn't — `backend/internal/platform/httpserver/server.go` has no CORS middleware today. Without it, the browser blocks every cross-origin credentialed request the frontend's refresh-cookie flow depends on (Vite dev server on `:5173` calling the API on `:8080`). This must land before Task 2 produces anything worth testing end-to-end.

**Files:**
- Modify: `backend/go.mod`, `backend/go.sum` (add `github.com/go-chi/cors`)
- Modify: `backend/internal/platform/httpserver/server.go`
- Modify: `backend/internal/platform/httpserver/server_test.go`
- Modify: `backend/internal/platform/config/config.go`
- Modify: `backend/internal/platform/config/config_test.go`
- Modify: `backend/cmd/api/main.go`
- Modify: `backend/.env.example`

**Interfaces:**
- Consumes: nothing new.
- Produces: `httpserver.NewRouter(allowedOrigin string) chi.Router` (signature change — was `NewRouter()`), `config.Config.CORSAllowedOrigin string` — consumed by `main.go` only within this task.

- [ ] **Step 1: Add the CORS dependency**

Run (from `backend/`):
```bash
go get github.com/go-chi/cors@latest
go mod edit -go=1.23 && go mod tidy
```
Confirm `head -3 go.mod` still shows `go 1.23` — this project's `go.mod` `go` directive has repeatedly been auto-bumped by `go mod tidy` toward whatever Go toolchain is installed locally; if it re-bumps, re-run `go mod edit -go=1.23` and manually verify `go.sum`/`go.mod` don't pull in an unrelated dependency upgrade.

- [ ] **Step 2: Write the failing test**

Add to `backend/internal/platform/config/config_test.go`:
```go
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
```

Replace `backend/internal/platform/httpserver/server_test.go` in full:
```go
package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	router := NewRouter("http://localhost:5173")

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

func TestCORS_PreflightAllowsConfiguredOrigin(t *testing.T) {
	router := NewRouter("http://localhost:5173")

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected Access-Control-Allow-Origin http://localhost:5173, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected Access-Control-Allow-Credentials true, got %q", got)
	}
}

func TestCORS_RejectsUnconfiguredOrigin(t *testing.T) {
	router := NewRouter("http://localhost:5173")

	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin for unconfigured origin, got %q", got)
	}
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run (from `backend/`): `go build ./... && go test ./internal/platform/... -v`
Expected: compile failure — `NewRouter` called with 1 arg doesn't match the current 0-arg signature; `cfg.CORSAllowedOrigin` undefined.

- [ ] **Step 4: Add `CORSAllowedOrigin` to config**

In `backend/internal/platform/config/config.go`, add the field to the `Config` struct:
```go
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
```
And in `Load()`, add it alongside the other `getEnv` calls:
```go
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
```
(Only the struct literal's field list changes — everything after it in `Load`, the validation checks and the `ACCESS_TOKEN_TTL_MINUTES` override block, stays exactly as-is.)

- [ ] **Step 5: Update `NewRouter` to take the allowed origin and mount CORS middleware**

Replace `backend/internal/platform/httpserver/server.go` in full:
```go
package httpserver

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(allowedOrigin string) chi.Router {
	r := chi.NewRouter()
	r.Use(redactWSToken, chimiddleware.Logger, chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{allowedOrigin},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Get("/healthz", healthHandler)
	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```
(`redactWSToken` is unchanged, defined in the sibling `logging.go` from Phase 4 — not touched by this task.)

- [ ] **Step 6: Wire the new config value into `main.go`**

In `backend/cmd/api/main.go`, change:
```go
	router := httpserver.NewRouter()
```
to:
```go
	router := httpserver.NewRouter(cfg.CORSAllowedOrigin)
```
(No other line in `main.go` changes — `cfg` is already in scope at that point in `main()`.)

- [ ] **Step 7: Document the new env var**

Append to `backend/.env.example`:
```
CORS_ALLOWED_ORIGIN=http://localhost:5173
```

- [ ] **Step 8: Run the tests and confirm they pass**

Run (from `backend/`): `go build ./... && go test ./... -race -v`
Expected: PASS, including the 4 new tests (`TestLoad_CORSAllowedOriginDefault`, `TestLoad_CORSAllowedOriginOverride`, `TestCORS_PreflightAllowsConfiguredOrigin`, `TestCORS_RejectsUnconfiguredOrigin`).

- [ ] **Step 9: Commit**

```bash
git add backend/go.mod backend/go.sum backend/internal/platform/httpserver/server.go backend/internal/platform/httpserver/server_test.go backend/internal/platform/config/config.go backend/internal/platform/config/config_test.go backend/cmd/api/main.go backend/.env.example
git commit -m "feat(backend): add CORS support for the frontend dev origin"
```

---

### Task 2: Scaffold the Vite + React + TypeScript + Tailwind + Vitest project

**Files:**
- Create: `frontend/` (via `npm create vite@latest`, then modified as below)
- Create: `frontend/vite.config.ts`
- Create: `frontend/src/lib/testSetup.ts`
- Create: `frontend/src/App.test.tsx`
- Modify: `frontend/src/App.tsx`, `frontend/src/index.css`
- Create: `frontend/.eslintrc.cjs` (or confirm the scaffold's ESLint config, adjusted below)
- Create: `.gitignore` entries for `frontend/node_modules`, `frontend/dist`

**Interfaces:**
- Produces: a working `npm run dev`, `npm test`, `npm run build`, `npm run lint` in `frontend/` — consumed by every later task in this plan and by Task 7's CI workflow.

**Note on Tailwind version:** the design spec's file tree (§2) lists a `tailwind.config.js`. This plan uses Tailwind CSS v4's `@tailwindcss/vite` plugin instead, which needs no config file for a minimal setup (theme customization, if ever needed, would add one back) — a one-line `@import "tailwindcss"` in `index.css` (Step 4 below) replaces both the config file and the old `@tailwind base/components/utilities` directives. This is a currently-standard setup detail, not a scope change.

- [ ] **Step 1: Scaffold with the official Vite template**

From the repo root:
```bash
npm create vite@latest frontend -- --template react-ts
cd frontend
npm install
```

- [ ] **Step 2: Add Tailwind CSS v4, React Router, Zustand, and test tooling**

From `frontend/`:
```bash
npm install tailwindcss @tailwindcss/vite react-router-dom zustand
npm install -D vitest jsdom @testing-library/react @testing-library/jest-dom @testing-library/user-event @vitest/coverage-v8
```

- [ ] **Step 3: Wire the Tailwind Vite plugin**

Edit `frontend/vite.config.ts` to add the Tailwind plugin and the Vitest config block:
```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: './src/lib/testSetup.ts',
  },
})
```

- [ ] **Step 4: Replace `frontend/src/index.css`'s contents**

```css
@import "tailwindcss";
```
(Delete any other CSS the scaffold generated in this file — Tailwind v4's single `@import` replaces the `@tailwind base/components/utilities` directives used in v3.)

- [ ] **Step 5: Add the Vitest setup file**

Create `frontend/src/lib/testSetup.ts`:
```ts
import '@testing-library/jest-dom/vitest'
```

- [ ] **Step 6: Add a `test` script to `package.json`**

In `frontend/package.json`, add to `"scripts"`:
```json
    "test": "vitest run"
```
(Keep the scaffold's existing `dev`, `build`, `lint`, `preview` scripts as-is — this only adds `test`.)

- [ ] **Step 7: Replace `frontend/src/App.tsx`'s contents with a minimal placeholder**

```tsx
function App() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <p className="text-lg text-gray-700">Kanvas</p>
    </div>
  )
}

export default App
```

- [ ] **Step 8: Write a smoke test proving the whole pipeline works**

Create `frontend/src/App.test.tsx`:
```tsx
import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'

describe('App', () => {
  it('renders the Kanvas placeholder', () => {
    render(<App />)
    expect(screen.getByText('Kanvas')).toBeInTheDocument()
  })
})
```

- [ ] **Step 9: Run the test suite and confirm it passes**

Run (from `frontend/`): `npm test`
Expected: PASS — 1 test.

- [ ] **Step 10: Run the linter and the build, confirm both succeed**

Run (from `frontend/`): `npm run lint && npm run build`
Expected: both exit 0. If `npm run lint` flags anything in the untouched scaffold files (e.g. `src/App.css` still present but unused after Step 4), delete the now-unused `src/App.css` and remove its import from `App.tsx` if the scaffold's `App.tsx` still imports it (Step 7 already replaced `App.tsx`'s contents, so this should be moot — verify).

- [ ] **Step 11: Add frontend build artifacts to the repo root `.gitignore`**

Confirm/append to the repo root `.gitignore`:
```
frontend/node_modules
frontend/dist
```

- [ ] **Step 12: Commit**

```bash
git add frontend .gitignore
git commit -m "feat(frontend): scaffold Vite + React + TypeScript + Tailwind + Vitest"
```

---

### Task 3: HTTP client with auth-header injection and 401-refresh-retry

**Files:**
- Create: `frontend/src/lib/env.ts`
- Create: `frontend/src/lib/env.test.ts`
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/api/client.test.ts`
- Create: `frontend/.env.example`

**Interfaces:**
- Consumes: nothing from earlier tasks (only the scaffold from Task 2).
- Produces: `env.API_URL: string` (`src/lib/env.ts`), `apiFetch<T>(path: string, options?: ApiFetchOptions) => Promise<T>` and `class ApiError extends Error { status: number; code: string }` and `setAccessToken(token: string | null): void` and `getAccessToken(): string | null` and `setUnauthorizedHandler(handler: () => void): void` (all from `src/api/client.ts`) — consumed by Task 4's `api/auth.ts` and by Task 4's `useAuthStore`.

- [ ] **Step 1: Write the failing test for `env.ts`**

Create `frontend/src/lib/env.test.ts`:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'

describe('env', () => {
  beforeEach(() => {
    vi.unstubAllEnvs()
    vi.resetModules()
  })

  it('throws when VITE_API_URL is not set', async () => {
    vi.stubEnv('VITE_API_URL', '')
    await expect(import('./env')).rejects.toThrow(/VITE_API_URL/)
  })

  it('exposes API_URL when VITE_API_URL is set', async () => {
    vi.stubEnv('VITE_API_URL', 'http://localhost:8080')
    const { env } = await import('./env')
    expect(env.API_URL).toBe('http://localhost:8080')
  })
})
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `frontend/`): `npm test -- env.test.ts`
Expected: FAIL — `./env` module doesn't exist yet.

- [ ] **Step 3: Implement `env.ts`**

Create `frontend/src/lib/env.ts`:
```ts
function requireEnv(key: string): string {
  const value = import.meta.env[key]
  if (!value) {
    throw new Error(`Missing required environment variable: ${key}`)
  }
  return value
}

export const env = {
  API_URL: requireEnv('VITE_API_URL'),
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run (from `frontend/`): `npm test -- env.test.ts`
Expected: PASS.

- [ ] **Step 5: Add `.env.example` and a local `.env` so `npm run dev`/other tests keep working**

Create `frontend/.env.example`:
```
VITE_API_URL=http://localhost:8080
```
Create `frontend/.env` (same content) — confirm the Vite scaffold's generated `.gitignore` inside `frontend/` already excludes `.env` (it does, by default, from `npm create vite`); if not, add `.env` to it, keeping `.env.example` tracked.

- [ ] **Step 6: Write the failing tests for `client.ts`**

Create `frontend/src/api/client.test.ts`:
```ts
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { apiFetch, ApiError, setAccessToken, getAccessToken, setUnauthorizedHandler } from './client'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('apiFetch', () => {
  beforeEach(() => {
    setAccessToken(null)
    setUnauthorizedHandler(() => {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('sends the Authorization header when an access token is set', async () => {
    setAccessToken('token-123')
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/boards')

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.Authorization).toBe('Bearer token-123')
    expect(init.credentials).toBe('include')
  })

  it('does not send an Authorization header when no token is set', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    await apiFetch('/auth/login', { method: 'POST' })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.headers.Authorization).toBeUndefined()
  })

  it('returns the parsed JSON body on success', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ hello: 'world' }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiFetch<{ hello: string }>('/boards')

    expect(result).toEqual({ hello: 'world' })
  })

  it('throws ApiError with code/message from the backend envelope on a non-401 error', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ error: { code: 'invalid_request', message: 'title is required' } }, 400),
    )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/boards', { method: 'POST' })).rejects.toMatchObject({
      status: 400,
      code: 'invalid_request',
      message: 'title is required',
    })
  })

  it('on a 401, retries once after a successful refresh, using the new token', async () => {
    setAccessToken('stale-token')
    const fetchMock = vi
      .fn()
      // 1st call: the original request, comes back 401
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'unauthorized', message: 'unauthorized' } }, 401))
      // 2nd call: POST /auth/refresh, succeeds with a new token
      .mockResolvedValueOnce(
        jsonResponse({ access_token: 'fresh-token', user: { id: '1', name: 'Ada', email: 'ada@example.com' } }),
      )
      // 3rd call: retry of the original request, succeeds
      .mockResolvedValueOnce(jsonResponse({ ok: true }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await apiFetch<{ ok: boolean }>('/boards')

    expect(result).toEqual({ ok: true })
    expect(fetchMock).toHaveBeenCalledTimes(3)
    expect(fetchMock.mock.calls[1][0]).toBe('http://localhost:8080/auth/refresh')
    expect(fetchMock.mock.calls[2][1].headers.Authorization).toBe('Bearer fresh-token')
    expect(getAccessToken()).toBe('fresh-token')
  })

  it('on a 401 where refresh also fails, calls the unauthorized handler and throws', async () => {
    setAccessToken('stale-token')
    const unauthorizedHandler = vi.fn()
    setUnauthorizedHandler(unauthorizedHandler)
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'unauthorized', message: 'unauthorized' } }, 401))
      .mockResolvedValueOnce(
        jsonResponse({ error: { code: 'invalid_refresh_token', message: 'invalid refresh token' } }, 401),
      )
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/boards')).rejects.toMatchObject({ status: 401 })
    expect(unauthorizedHandler).toHaveBeenCalledTimes(1)
    expect(getAccessToken()).toBeNull()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('does not attempt a refresh loop when the failing request IS the refresh call', async () => {
    setAccessToken('stale-token')
    const unauthorizedHandler = vi.fn()
    setUnauthorizedHandler(unauthorizedHandler)
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'invalid_refresh_token', message: 'bad' } }, 401))
    vi.stubGlobal('fetch', fetchMock)

    await expect(apiFetch('/auth/refresh', { method: 'POST', skipAuthRetry: true })).rejects.toMatchObject({
      status: 401,
    })
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(unauthorizedHandler).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 7: Run the tests and confirm they fail**

Run (from `frontend/`): `npm test -- client.test.ts`
Expected: FAIL — `./client` module doesn't exist.

- [ ] **Step 8: Implement `client.ts`**

Create `frontend/src/api/client.ts`:
```ts
import { env } from '../lib/env'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

let accessToken: string | null = null
let unauthorizedHandler: () => void = () => {}

export function setAccessToken(token: string | null): void {
  accessToken = token
}

export function getAccessToken(): string | null {
  return accessToken
}

export function setUnauthorizedHandler(handler: () => void): void {
  unauthorizedHandler = handler
}

interface ApiFetchOptions extends Omit<RequestInit, 'body'> {
  body?: unknown
  /** Skip the 401-refresh-retry cycle — used by the refresh call itself to avoid recursion. */
  skipAuthRetry?: boolean
}

interface BackendErrorBody {
  error?: { code?: string; message?: string }
}

async function parseErrorBody(response: Response): Promise<BackendErrorBody> {
  try {
    return await response.json()
  } catch {
    return {}
  }
}

async function performFetch(path: string, options: ApiFetchOptions): Promise<Response> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(options.headers as Record<string, string> | undefined),
  }
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`
  }

  return fetch(`${env.API_URL}${path}`, {
    ...options,
    headers,
    credentials: 'include',
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  })
}

export async function apiFetch<T>(path: string, options: ApiFetchOptions = {}): Promise<T> {
  const response = await performFetch(path, options)

  if (response.status === 401 && !options.skipAuthRetry) {
    const refreshed = await tryRefresh()
    if (refreshed) {
      const retryResponse = await performFetch(path, options)
      return handleResponse<T>(retryResponse)
    }
    unauthorizedHandler()
  }

  return handleResponse<T>(response)
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const body = await parseErrorBody(response)
    throw new ApiError(
      response.status,
      body.error?.code ?? 'unknown_error',
      body.error?.message ?? response.statusText,
    )
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

async function tryRefresh(): Promise<boolean> {
  const response = await performFetch('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  if (!response.ok) {
    setAccessToken(null)
    return false
  }
  const body = (await response.json()) as { access_token: string }
  setAccessToken(body.access_token)
  return true
}
```

- [ ] **Step 9: Run the tests and confirm they pass**

Run (from `frontend/`): `npm test -- client.test.ts`
Expected: PASS — all 7 tests.

- [ ] **Step 10: Run the full frontend test suite**

Run (from `frontend/`): `npm test`
Expected: PASS — all tests across `App.test.tsx`, `env.test.ts`, `client.test.ts`.

- [ ] **Step 11: Commit**

```bash
git add frontend/src/lib/env.ts frontend/src/lib/env.test.ts frontend/src/api/client.ts frontend/src/api/client.test.ts frontend/.env.example
git commit -m "feat(frontend): add HTTP client with auth header injection and 401 refresh retry"
```

---

### Task 4: Auth API functions and Zustand session store

**Files:**
- Create: `frontend/src/api/auth.ts`
- Create: `frontend/src/api/auth.test.ts`
- Create: `frontend/src/features/auth/useAuthStore.ts`
- Create: `frontend/src/features/auth/useAuthStore.test.ts`

**Interfaces:**
- Consumes: `apiFetch`, `ApiError`, `setAccessToken`, `getAccessToken`, `setUnauthorizedHandler` from `../../api/client` (Task 3).
- Produces: `registerUser`, `loginUser`, `refreshSession`, `logoutUser` (all `../../api/auth`, each `(args) => Promise<AuthResult>` except `logoutUser: () => Promise<void>`), `type AuthResult = { user: User; accessToken: string }`, `type User = { id: string; name: string; email: string }` — consumed by Task 5's `LoginPage`/`RegisterPage` and Task 6's `App.tsx`/`RequireAuth`. `useAuthStore` (Zustand hook) exposing `{ user: User | null; status: 'idle' | 'authenticated' | 'unauthenticated'; login(email, password): Promise<void>; register(name, email, password): Promise<void>; logout(): Promise<void>; restoreSession(): Promise<void> }` — consumed by Task 5 and Task 6.

- [ ] **Step 1: Write the failing tests for `auth.ts`**

Create `frontend/src/api/auth.test.ts`:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { registerUser, loginUser, refreshSession, logoutUser } from './auth'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

describe('auth API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('registerUser posts to /auth/register and returns the auth result', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    const result = await registerUser('Ada', 'ada@example.com', 'password123')

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/register', {
      method: 'POST',
      body: { name: 'Ada', email: 'ada@example.com', password: 'password123' },
      skipAuthRetry: true,
    })
    expect(result).toEqual({
      accessToken: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })
  })

  it('loginUser posts to /auth/login and returns the auth result', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    const result = await loginUser('ada@example.com', 'password123')

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/login', {
      method: 'POST',
      body: { email: 'ada@example.com', password: 'password123' },
      skipAuthRetry: true,
    })
    expect(result.user.email).toBe('ada@example.com')
  })

  it('refreshSession posts to /auth/refresh with skipAuthRetry', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue({
      access_token: 'tok',
      user: { id: '1', name: 'Ada', email: 'ada@example.com' },
    })

    await refreshSession()

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  })

  it('logoutUser posts to /auth/logout with skipAuthRetry', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await logoutUser()

    expect(client.apiFetch).toHaveBeenCalledWith('/auth/logout', { method: 'POST', skipAuthRetry: true })
  })
})
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `frontend/`): `npm test -- api/auth.test.ts`
Expected: FAIL — `./auth` module doesn't exist.

- [ ] **Step 3: Implement `auth.ts`**

Create `frontend/src/api/auth.ts`:
```ts
import { apiFetch } from './client'

export interface User {
  id: string
  name: string
  email: string
}

export interface AuthResult {
  user: User
  accessToken: string
}

interface AuthResponseBody {
  access_token: string
  user: User
}

function toAuthResult(body: AuthResponseBody): AuthResult {
  return { user: body.user, accessToken: body.access_token }
}

export async function registerUser(name: string, email: string, password: string): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/register', {
    method: 'POST',
    body: { name, email, password },
    skipAuthRetry: true,
  })
  return toAuthResult(body)
}

export async function loginUser(email: string, password: string): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/login', {
    method: 'POST',
    body: { email, password },
    skipAuthRetry: true,
  })
  return toAuthResult(body)
}

export async function refreshSession(): Promise<AuthResult> {
  const body = await apiFetch<AuthResponseBody>('/auth/refresh', { method: 'POST', skipAuthRetry: true })
  return toAuthResult(body)
}

export async function logoutUser(): Promise<void> {
  await apiFetch<void>('/auth/logout', { method: 'POST', skipAuthRetry: true })
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `frontend/`): `npm test -- api/auth.test.ts`
Expected: PASS — all 4 tests.

- [ ] **Step 5: Write the failing tests for `useAuthStore`**

Create `frontend/src/features/auth/useAuthStore.test.ts`:
```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { useAuthStore } from './useAuthStore'
import * as authApi from '../../api/auth'
import { ApiError } from '../../api/client'

vi.mock('../../api/auth')

const sampleUser = { id: '1', name: 'Ada', email: 'ada@example.com' }

describe('useAuthStore', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
    vi.resetAllMocks()
  })

  it('login: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.loginUser).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().login('ada@example.com', 'password123')

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('login: on failure, leaves status unauthenticated and rethrows', async () => {
    vi.mocked(authApi.loginUser).mockRejectedValue(new ApiError(401, 'invalid_credentials', 'invalid credentials'))

    await expect(useAuthStore.getState().login('ada@example.com', 'wrong')).rejects.toThrow('invalid credentials')

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('register: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.registerUser).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().register('Ada', 'ada@example.com', 'password123')

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('restoreSession: on success, sets user and status to authenticated', async () => {
    vi.mocked(authApi.refreshSession).mockResolvedValue({ user: sampleUser, accessToken: 'tok' })

    await useAuthStore.getState().restoreSession()

    expect(useAuthStore.getState().user).toEqual(sampleUser)
    expect(useAuthStore.getState().status).toBe('authenticated')
  })

  it('restoreSession: on failure, sets status to unauthenticated without throwing', async () => {
    vi.mocked(authApi.refreshSession).mockRejectedValue(new ApiError(401, 'invalid_refresh_token', 'no session'))

    await expect(useAuthStore.getState().restoreSession()).resolves.toBeUndefined()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('logout: clears user and sets status to unauthenticated', async () => {
    useAuthStore.setState({ user: sampleUser, status: 'authenticated' })
    vi.mocked(authApi.logoutUser).mockResolvedValue(undefined)

    await useAuthStore.getState().logout()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })

  it('logout: still clears local state even if the backend call fails', async () => {
    useAuthStore.setState({ user: sampleUser, status: 'authenticated' })
    vi.mocked(authApi.logoutUser).mockRejectedValue(new Error('network error'))

    await useAuthStore.getState().logout()

    expect(useAuthStore.getState().user).toBeNull()
    expect(useAuthStore.getState().status).toBe('unauthenticated')
  })
})
```

- [ ] **Step 6: Run the tests and confirm they fail**

Run (from `frontend/`): `npm test -- useAuthStore.test.ts`
Expected: FAIL — `./useAuthStore` module doesn't exist.

- [ ] **Step 7: Implement `useAuthStore.ts`**

Create `frontend/src/features/auth/useAuthStore.ts`:
```ts
import { create } from 'zustand'
import { loginUser, registerUser, refreshSession, logoutUser, type User } from '../../api/auth'
import { setAccessToken } from '../../api/client'

export type AuthStatus = 'idle' | 'authenticated' | 'unauthenticated'

interface AuthState {
  user: User | null
  status: AuthStatus
  login: (email: string, password: string) => Promise<void>
  register: (name: string, email: string, password: string) => Promise<void>
  restoreSession: () => Promise<void>
  logout: () => Promise<void>
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  status: 'idle',

  login: async (email, password) => {
    try {
      const result = await loginUser(email, password)
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch (error) {
      set({ user: null, status: 'unauthenticated' })
      throw error
    }
  },

  register: async (name, email, password) => {
    try {
      const result = await registerUser(name, email, password)
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch (error) {
      set({ user: null, status: 'unauthenticated' })
      throw error
    }
  },

  restoreSession: async () => {
    try {
      const result = await refreshSession()
      setAccessToken(result.accessToken)
      set({ user: result.user, status: 'authenticated' })
    } catch {
      setAccessToken(null)
      set({ user: null, status: 'unauthenticated' })
    }
  },

  logout: async () => {
    try {
      await logoutUser()
    } finally {
      setAccessToken(null)
      set({ user: null, status: 'unauthenticated' })
    }
  },
}))
```

- [ ] **Step 8: Run the tests and confirm they pass**

Run (from `frontend/`): `npm test -- useAuthStore.test.ts`
Expected: PASS — all 7 tests.

- [ ] **Step 9: Run the full frontend test suite**

Run (from `frontend/`): `npm test`
Expected: PASS across all files so far.

- [ ] **Step 10: Commit**

```bash
git add frontend/src/api/auth.ts frontend/src/api/auth.test.ts frontend/src/features/auth/useAuthStore.ts frontend/src/features/auth/useAuthStore.test.ts
git commit -m "feat(frontend): add auth API functions and Zustand session store"
```

---

### Task 5: Login and Register pages

**Files:**
- Create: `frontend/src/features/auth/LoginPage.tsx`
- Create: `frontend/src/features/auth/LoginPage.test.tsx`
- Create: `frontend/src/features/auth/RegisterPage.tsx`
- Create: `frontend/src/features/auth/RegisterPage.test.tsx`

**Interfaces:**
- Consumes: `useAuthStore` (Task 4), `ApiError` from `../../api/client` (Task 3). Both pages call `useNavigate`/`Link` from `react-router-dom` (used here for the first time in the codebase — Task 6 provides the actual `<Router>` these run inside; these component tests wrap each page in a `MemoryRouter` directly, so they don't depend on Task 6).
- Produces: `LoginPage` and `RegisterPage` (default exports) — consumed by Task 6's `router.tsx`.

- [ ] **Step 1: Write the failing tests for `LoginPage`**

Create `frontend/src/features/auth/LoginPage.test.tsx`:
```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import LoginPage from './LoginPage'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

describe('LoginPage', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'unauthenticated', login: vi.fn() })
  })

  it('requires email and password before submitting', async () => {
    const login = vi.fn()
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(login).not.toHaveBeenCalled()
    expect(screen.getByLabelText(/e-mail/i)).toBeInvalid()
  })

  it('calls login with the entered credentials on submit', async () => {
    const login = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    await waitFor(() => expect(login).toHaveBeenCalledWith('ada@example.com', 'password123'))
  })

  it('redirects back to the originally-requested URL after login, when one was preserved', async () => {
    const login = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ login })
    render(
      <MemoryRouter
        initialEntries={[{ pathname: '/login', state: { from: { pathname: '/boards/42' } } }]}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/boards/42" element={<p>Board 42</p>} />
        </Routes>
      </MemoryRouter>,
    )

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(await screen.findByText('Board 42')).toBeInTheDocument()
  })

  it('shows the backend error message when login fails', async () => {
    const login = vi.fn().mockRejectedValue(new ApiError(401, 'invalid_credentials', 'invalid email or password'))
    useAuthStore.setState({ login })
    render(<LoginPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'wrong-password')
    await userEvent.click(screen.getByRole('button', { name: /entrar/i }))

    expect(await screen.findByText('invalid email or password')).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `frontend/`): `npm test -- LoginPage.test.tsx`
Expected: FAIL — `./LoginPage` module doesn't exist.

- [ ] **Step 3: Implement `LoginPage.tsx`**

Create `frontend/src/features/auth/LoginPage.tsx`:
```tsx
import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useLocation } from 'react-router-dom'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

interface LocationState {
  from?: { pathname: string }
}

export default function LoginPage() {
  const login = useAuthStore((state) => state.login)
  const navigate = useNavigate()
  const location = useLocation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(email, password)
      const redirectTo = (location.state as LocationState | null)?.from?.pathname ?? '/'
      navigate(redirectTo, { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Não foi possível conectar ao servidor.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-4 rounded-lg bg-white p-8 shadow">
        <h1 className="text-xl font-semibold text-gray-900">Entrar no Kanvas</h1>

        {error && <p className="rounded bg-red-50 p-2 text-sm text-red-700">{error}</p>}

        <div>
          <label htmlFor="email" className="block text-sm font-medium text-gray-700">
            E-mail
          </label>
          <input
            id="email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>

        <div>
          <label htmlFor="password" className="block text-sm font-medium text-gray-700">
            Senha
          </label>
          <input
            id="password"
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="w-full rounded bg-blue-600 py-2 text-white disabled:opacity-50"
        >
          Entrar
        </button>

        <p className="text-center text-sm text-gray-600">
          Não tem conta?{' '}
          <Link to="/register" className="text-blue-600">
            Criar conta
          </Link>
        </p>
      </form>
    </div>
  )
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run (from `frontend/`): `npm test -- LoginPage.test.tsx`
Expected: PASS — all 4 tests.

- [ ] **Step 5: Write the failing tests for `RegisterPage`**

Create `frontend/src/features/auth/RegisterPage.test.tsx`:
```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import RegisterPage from './RegisterPage'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

describe('RegisterPage', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'unauthenticated', register: vi.fn() })
  })

  it('requires name, email and password before submitting', async () => {
    const register = vi.fn()
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    expect(register).not.toHaveBeenCalled()
  })

  it('calls register with the entered data on submit', async () => {
    const register = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/nome/i), 'Ada Lovelace')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    await waitFor(() =>
      expect(register).toHaveBeenCalledWith('Ada Lovelace', 'ada@example.com', 'password123'),
    )
  })

  it('shows the backend error message when registration fails', async () => {
    const register = vi.fn().mockRejectedValue(new ApiError(409, 'email_taken', 'email already registered'))
    useAuthStore.setState({ register })
    render(<RegisterPage />, { wrapper: MemoryRouter })

    await userEvent.type(screen.getByLabelText(/nome/i), 'Ada Lovelace')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.type(screen.getByLabelText(/senha/i), 'password123')
    await userEvent.click(screen.getByRole('button', { name: /criar conta/i }))

    expect(await screen.findByText('email already registered')).toBeInTheDocument()
  })
})
```

- [ ] **Step 6: Run the test and confirm it fails**

Run (from `frontend/`): `npm test -- RegisterPage.test.tsx`
Expected: FAIL — `./RegisterPage` module doesn't exist.

- [ ] **Step 7: Implement `RegisterPage.tsx`**

Create `frontend/src/features/auth/RegisterPage.tsx`:
```tsx
import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuthStore } from './useAuthStore'
import { ApiError } from '../../api/client'

export default function RegisterPage() {
  const register = useAuthStore((state) => state.register)
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await register(name, email, password)
      navigate('/', { replace: true })
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message)
      } else {
        setError('Não foi possível conectar ao servidor.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-gray-50">
      <form onSubmit={handleSubmit} className="w-full max-w-sm space-y-4 rounded-lg bg-white p-8 shadow">
        <h1 className="text-xl font-semibold text-gray-900">Criar conta no Kanvas</h1>

        {error && <p className="rounded bg-red-50 p-2 text-sm text-red-700">{error}</p>}

        <div>
          <label htmlFor="name" className="block text-sm font-medium text-gray-700">
            Nome
          </label>
          <input
            id="name"
            type="text"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>

        <div>
          <label htmlFor="email" className="block text-sm font-medium text-gray-700">
            E-mail
          </label>
          <input
            id="email"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>

        <div>
          <label htmlFor="password" className="block text-sm font-medium text-gray-700">
            Senha
          </label>
          <input
            id="password"
            type="password"
            required
            minLength={8}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="w-full rounded bg-blue-600 py-2 text-white disabled:opacity-50"
        >
          Criar conta
        </button>

        <p className="text-center text-sm text-gray-600">
          Já tem conta?{' '}
          <Link to="/login" className="text-blue-600">
            Entrar
          </Link>
        </p>
      </form>
    </div>
  )
}
```

- [ ] **Step 8: Run the test and confirm it passes**

Run (from `frontend/`): `npm test -- RegisterPage.test.tsx`
Expected: PASS — all 3 tests.

- [ ] **Step 9: Install `react-router-dom` types check and run the full suite**

Run (from `frontend/`): `npm test && npm run build`
Expected: both succeed (the build step also catches any TypeScript error `npm test` alone wouldn't, since Vitest doesn't type-check by default).

- [ ] **Step 10: Commit**

```bash
git add frontend/src/features/auth/LoginPage.tsx frontend/src/features/auth/LoginPage.test.tsx frontend/src/features/auth/RegisterPage.tsx frontend/src/features/auth/RegisterPage.test.tsx
git commit -m "feat(frontend): add login and register pages"
```

---

### Task 6: Routing, protected routes, layout, and app wiring

**Files:**
- Create: `frontend/src/routes/RequireAuth.tsx`
- Create: `frontend/src/routes/RequireAuth.test.tsx`
- Create: `frontend/src/components/layout/AppLayout.tsx`
- Create: `frontend/src/components/layout/AppLayout.test.tsx`
- Create: `frontend/src/routes/router.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`
- Modify: `frontend/src/main.tsx`

**Interfaces:**
- Consumes: `useAuthStore` (Task 4), `LoginPage`/`RegisterPage` (Task 5), `setUnauthorizedHandler` (Task 3).
- Produces: `RequireAuth` (wraps `<Outlet />`), `AppLayout` (wraps `<Outlet />` with header + logout button), `router` (a `createBrowserRouter` instance, default export from `router.tsx`) — nothing later in this plan consumes these (this is the last task), but this is the shape Phase 6 will extend with board routes nested under the same `AppLayout`.

- [ ] **Step 1: Write the failing tests for `RequireAuth`**

Create `frontend/src/routes/RequireAuth.test.tsx`:
```tsx
import { describe, expect, it, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import { useAuthStore } from '../features/auth/useAuthStore'

function renderWithAuth(status: 'idle' | 'authenticated' | 'unauthenticated') {
  useAuthStore.setState({ status })
  return render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route element={<RequireAuth />}>
          <Route path="/" element={<p>Protected content</p>} />
        </Route>
        <Route path="/login" element={<p>Login page</p>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('RequireAuth', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
  })

  it('renders the protected content when authenticated', () => {
    renderWithAuth('authenticated')
    expect(screen.getByText('Protected content')).toBeInTheDocument()
  })

  it('redirects to /login when unauthenticated', () => {
    renderWithAuth('unauthenticated')
    expect(screen.getByText('Login page')).toBeInTheDocument()
  })

  it('renders nothing (no redirect yet) while status is idle', () => {
    renderWithAuth('idle')
    expect(screen.queryByText('Protected content')).not.toBeInTheDocument()
    expect(screen.queryByText('Login page')).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `frontend/`): `npm test -- RequireAuth.test.tsx`
Expected: FAIL — `./RequireAuth` module doesn't exist.

- [ ] **Step 3: Implement `RequireAuth.tsx`**

Create `frontend/src/routes/RequireAuth.tsx`:
```tsx
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useAuthStore } from '../features/auth/useAuthStore'

export default function RequireAuth() {
  const status = useAuthStore((state) => state.status)
  const location = useLocation()

  if (status === 'idle') {
    return null
  }

  if (status === 'unauthenticated') {
    return <Navigate to="/login" state={{ from: location }} replace />
  }

  return <Outlet />
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run (from `frontend/`): `npm test -- RequireAuth.test.tsx`
Expected: PASS — all 3 tests.

- [ ] **Step 5: Write the failing tests for `AppLayout`**

Create `frontend/src/components/layout/AppLayout.test.tsx`:
```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import AppLayout from './AppLayout'
import { useAuthStore } from '../../features/auth/useAuthStore'

describe('AppLayout', () => {
  beforeEach(() => {
    useAuthStore.setState({
      user: { id: '1', name: 'Ada Lovelace', email: 'ada@example.com' },
      status: 'authenticated',
      logout: vi.fn(),
    })
  })

  it("shows the user's name and the wrapped route content", () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/" element={<p>Home content</p>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
    expect(screen.getByText('Home content')).toBeInTheDocument()
  })

  it('calls logout when the logout button is clicked', async () => {
    const logout = vi.fn().mockResolvedValue(undefined)
    useAuthStore.setState({ logout })
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="/" element={<p>Home content</p>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

    await userEvent.click(screen.getByRole('button', { name: /sair/i }))

    expect(logout).toHaveBeenCalledTimes(1)
  })
})
```

- [ ] **Step 6: Run the test and confirm it fails**

Run (from `frontend/`): `npm test -- AppLayout.test.tsx`
Expected: FAIL — `./AppLayout` module doesn't exist.

- [ ] **Step 7: Implement `AppLayout.tsx`**

Create `frontend/src/components/layout/AppLayout.tsx`:
```tsx
import { Outlet } from 'react-router-dom'
import { useAuthStore } from '../../features/auth/useAuthStore'

export default function AppLayout() {
  const user = useAuthStore((state) => state.user)
  const logout = useAuthStore((state) => state.logout)

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="flex items-center justify-between border-b bg-white px-6 py-3">
        <span className="font-semibold text-gray-900">Kanvas</span>
        <div className="flex items-center gap-4">
          {user && <span className="text-sm text-gray-700">{user.name}</span>}
          <button
            type="button"
            onClick={() => logout()}
            className="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-100"
          >
            Sair
          </button>
        </div>
      </header>
      <main className="p-6">
        <Outlet />
      </main>
    </div>
  )
}
```

- [ ] **Step 8: Run the test and confirm it passes**

Run (from `frontend/`): `npm test -- AppLayout.test.tsx`
Expected: PASS — both tests.

- [ ] **Step 9: Replace `App.tsx` with the placeholder home page + session-restore wiring**

Replace `frontend/src/App.tsx` in full:
```tsx
import { useEffect } from 'react'
import { RouterProvider } from 'react-router-dom'
import { router } from './routes/router'
import { useAuthStore } from './features/auth/useAuthStore'
import { setUnauthorizedHandler } from './api/client'

export default function App() {
  const restoreSession = useAuthStore((state) => state.restoreSession)
  const status = useAuthStore((state) => state.status)

  useEffect(() => {
    setUnauthorizedHandler(() => {
      useAuthStore.setState({ user: null, status: 'unauthenticated' })
    })
    restoreSession()
  }, [restoreSession])

  if (status === 'idle') {
    return (
      <div className="flex min-h-screen items-center justify-center text-gray-500">
        Carregando...
      </div>
    )
  }

  return <RouterProvider router={router} />
}
```

- [ ] **Step 10: Create the home placeholder page and `router.tsx`**

Create `frontend/src/routes/router.tsx`:
```tsx
import { createBrowserRouter, Navigate } from 'react-router-dom'
import RequireAuth from './RequireAuth'
import AppLayout from '../components/layout/AppLayout'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import { useAuthStore } from '../features/auth/useAuthStore'

function HomePage() {
  const user = useAuthStore((state) => state.user)
  return <p className="text-gray-700">Bem-vindo, {user?.name}. A lista de boards chega na próxima fase.</p>
}

function RedirectIfAuthenticated({ children }: { children: React.ReactNode }) {
  const status = useAuthStore((state) => state.status)
  if (status === 'authenticated') {
    return <Navigate to="/" replace />
  }
  return <>{children}</>
}

export const router = createBrowserRouter([
  {
    path: '/login',
    element: (
      <RedirectIfAuthenticated>
        <LoginPage />
      </RedirectIfAuthenticated>
    ),
  },
  {
    path: '/register',
    element: (
      <RedirectIfAuthenticated>
        <RegisterPage />
      </RedirectIfAuthenticated>
    ),
  },
  {
    element: <RequireAuth />,
    children: [
      {
        element: <AppLayout />,
        children: [{ path: '/', element: <HomePage /> }],
      },
    ],
  },
])
```

- [ ] **Step 11: Update `main.tsx` (remove `<React.StrictMode>`'s direct `<App />` render if the scaffold wrapped differently — keep it simple)**

Confirm `frontend/src/main.tsx` matches (adjust only if the Vite scaffold generated something different — the router now lives inside `App`, so `main.tsx` needs no router-specific changes):
```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
```

- [ ] **Step 12: Replace `App.test.tsx`'s smoke test (the old placeholder text no longer renders directly)**

Replace `frontend/src/App.test.tsx` in full:
```tsx
import { describe, expect, it, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import App from './App'
import { useAuthStore } from './features/auth/useAuthStore'

describe('App', () => {
  beforeEach(() => {
    useAuthStore.setState({ user: null, status: 'idle' })
  })

  it('shows a loading state while session restore is in flight', () => {
    render(<App />)
    expect(screen.getByText('Carregando...')).toBeInTheDocument()
  })
})
```
(This test only covers the `idle` loading state without mocking `restoreSession`'s network call — a fuller router-integration test exercising the authenticated/unauthenticated paths end-to-end is reasonable follow-up work, not required to close out this phase, since `RequireAuth`, `AppLayout`, `LoginPage`, and `RegisterPage` are already tested individually and `restoreSession`'s success/failure branches are already tested in Task 4's `useAuthStore.test.ts`.)

- [ ] **Step 13: Run the full frontend test suite**

Run (from `frontend/`): `npm test`
Expected: PASS across every test file in the project.

- [ ] **Step 14: Run the linter and the build**

Run (from `frontend/`): `npm run lint && npm run build`
Expected: both exit 0.

- [ ] **Step 15: Manual smoke test against the real backend**

With the backend running (`docker compose up -d --build` from the repo root, or `make run` per `backend/README.md`) and `frontend/.env` pointing `VITE_API_URL` at it:
```bash
cd frontend
npm run dev
```
Open the printed local URL, register a new user, confirm redirect to the home placeholder showing the user's name, reload the page (confirms silent session restore via the refresh cookie), then click "Sair" and confirm redirect back to `/login`. This step has no automated assertion — it's a manual pass proving the CORS configuration from Task 1 actually works end-to-end, which no unit test in this plan exercises (they all mock `fetch`).

- [ ] **Step 16: Commit**

```bash
git add frontend/src/routes frontend/src/components frontend/src/App.tsx frontend/src/App.test.tsx frontend/src/main.tsx
git commit -m "feat(frontend): add routing, protected routes, and app layout"
```

---

### Task 7: Frontend Dockerfile, CI workflow, and documentation

**Files:**
- Create: `frontend/Dockerfile`
- Create: `frontend/nginx.conf`
- Create: `frontend/.dockerignore`
- Create: `.github/workflows/frontend-ci.yml`
- Create: `frontend/README.md`
- Modify: root `README.md`

**Interfaces:**
- Consumes: nothing new (this task packages/documents what Tasks 1-6 already built).
- Produces: nothing consumed later in this plan — this is the last task of Phase 5.

- [ ] **Step 1: Add the frontend Dockerfile**

Create `frontend/Dockerfile`:
```dockerfile
# --- build stage ---
FROM node:20-alpine AS build
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}
RUN npm run build

# --- serve stage ---
FROM nginx:1.27-alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

- [ ] **Step 2: Add the Nginx config (SPA fallback routing)**

Create `frontend/nginx.conf`:
```nginx
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```
(`try_files ... /index.html` is required because React Router does client-side routing — without this, refreshing on `/login` or `/register` would 404 at the Nginx level.)

- [ ] **Step 3: Add `.dockerignore`**

Create `frontend/.dockerignore`:
```
node_modules
dist
.env
```

- [ ] **Step 4: Add the frontend CI workflow**

Create `.github/workflows/frontend-ci.yml`:
```yaml
name: frontend-ci

on:
  push:
    paths:
      - "frontend/**"
      - ".github/workflows/frontend-ci.yml"
  pull_request:
    paths:
      - "frontend/**"
      - ".github/workflows/frontend-ci.yml"

jobs:
  test:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: frontend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "npm"
          cache-dependency-path: frontend/package-lock.json
      - run: npm ci
      - run: npm run lint
      - name: Create .env for build/test
        run: echo "VITE_API_URL=http://localhost:8080" > .env
      - run: npm test
      - run: npm run build
```
(Mirrors `backend-ci.yml`'s path-filtered trigger and single-job structure. `VITE_API_URL` is written to a throwaway `.env` in CI purely so `npm run build`/`npm test` don't fail on `env.ts`'s fail-fast check — no real backend is contacted in CI, since every test mocks `fetch`.)

- [ ] **Step 5: Write `frontend/README.md`**

Create `frontend/README.md`:
```markdown
# Kanvas frontend

React + TypeScript SPA for Kanvas — Vite, Tailwind CSS, React Router, Zustand.

## Prerequisites

- Node.js 20+
- The Kanvas backend running locally (see `../backend/README.md`) with CORS configured for this app's origin (`CORS_ALLOWED_ORIGIN`, defaults to `http://localhost:5173`)

## Local development

    cp .env.example .env   # defaults to VITE_API_URL=http://localhost:8080
    npm install
    npm run dev

Open the printed local URL (usually `http://localhost:5173`).

## Testing

    npm test          # Vitest + Testing Library, all fetch calls mocked
    npm run lint
    npm run build

## Authentication

- The access token lives in memory only (Zustand store, `src/features/auth/useAuthStore.ts`) — never in `localStorage`, to limit XSS exposure.
- The refresh token is an `httpOnly` cookie set by the backend; the frontend never reads it directly. `src/api/client.ts` sends `credentials: "include"` on every request so the browser attaches/receives it automatically.
- On a `401`, `src/api/client.ts` transparently calls `/auth/refresh` once and retries the original request; if that also fails, the user is signed out and redirected to `/login`.

## Project layout

    src/
      api/          HTTP client (auth header injection, 401 refresh-retry) and auth API functions
      features/
        auth/        login/register pages, Zustand session store
      routes/        React Router setup, protected-route guard
      components/
        layout/      authenticated app shell (header, logout)
      lib/           env var loading, test setup
```

- [ ] **Step 6: Update the root `README.md`**

Replace the root `README.md`'s "Status" line and frontend bullet — read the current file first (`README.md` at repo root) and update it to reflect Phase 5's completion, following this shape:
```markdown
- **Frontend:** React + TypeScript (Vite, Tailwind, React Router, Zustand) — see [`frontend/README.md`](frontend/README.md)

## Status

Phases 1-5 complete: backend foundation, authentication, boards & members, columns & cards, realtime WebSocket, and the frontend's authentication flow (register/login/logout with automatic token refresh). See [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for implementation plans by phase.
```
(Keep the rest of the root README — the backend bullet line, any other existing content — unchanged; only the frontend bullet and the "Status" section's text change.)

- [ ] **Step 7: Verify the full stack still boots via Docker Compose (manual, no automated assertion)**

This step doesn't modify `docker-compose.yml` — Phase 5 doesn't wire the frontend container into it yet (docker-compose wiring for the frontend service is reasonable to fold into Phase 8's CI/CD work, since that's when the deploy-shaped Docker setup gets finalized). Skip this step if you don't have time to verify manually; it's a nice-to-have confirmation, not a blocking check: `docker build -t kanvas-frontend --build-arg VITE_API_URL=http://localhost:8080 ./frontend` should succeed.

- [ ] **Step 8: Commit**

```bash
git add frontend/Dockerfile frontend/nginx.conf frontend/.dockerignore .github/workflows/frontend-ci.yml frontend/README.md README.md
git commit -m "feat(frontend): add Dockerfile, CI workflow, and documentation"
```

---

## Definition of Done

- `cd backend && go build ./... && go test ./... -race && go test ./... -race -tags=integration` all pass (Task 1's CORS changes don't break anything already covered by Phase 1-4's suites).
- `cd frontend && npm test && npm run lint && npm run build` all pass.
- Manual smoke test (Task 6, Step 15) confirms register → redirect → reload-restores-session → logout works end-to-end against the real backend, proving Task 1's CORS wiring is correct (no unit test in this plan makes a real network call).
- Both `backend-ci.yml` and the new `frontend-ci.yml` are green on the pushed branch.
- Root `README.md` and `frontend/README.md` are up to date.
