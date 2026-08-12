# Kanvas Phase 6: Boards UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the frontend boards experience (list, kanban view, drag-and-drop, members, realtime) on top of the existing Kanvas backend, plus a backend hardening fix for the WebSocket Origin check and a small backend addition so the members list carries name/email.

**Architecture:** React Query (`@tanstack/react-query`) owns all board/column/card server state; Zustand stays scoped to auth only. `@dnd-kit/core` + `@dnd-kit/sortable` power drag-and-drop for cards and columns. A WebSocket connection per open board patches the React Query cache directly from realtime events instead of refetching.

**Tech Stack:** React 19, TypeScript, Vite 6, Tailwind v4, React Router v7, `@tanstack/react-query`, `@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities`, Vitest + Testing Library. Backend: Go 1.23, chi, `github.com/coder/websocket`, sqlc.

## Global Constraints

- All frontend API calls go through the existing `apiFetch` in `frontend/src/api/client.ts` — never call `fetch` directly from a new module.
- Wire-format request/response bodies are snake_case; every `api/*.ts` module maps them to camelCase domain types at the module boundary, exactly like `frontend/src/api/auth.ts` already does (`AuthResponseBody` → `AuthResult` via `toAuthResult`).
- Error handling relies on the existing `ApiError` class (`status`, `code`, `message`) thrown by `apiFetch` — do not introduce a second error type.
- Frontend tests use Vitest + Testing Library, mock `api/*.ts` modules (never mock `fetch` directly in a component test), and assert behavior/rendered output, not implementation details.
- Backend tests follow existing conventions: unit tests against fakes run in the default `go test ./...`; tests tagged `//go:build integration` run only against a real Postgres and are not part of the standard suite run during this plan's task reviews (do not add new integration-only requirements to the default test command).
- Every new/changed file must pass `npm run lint` (frontend) or `go vet ./...` (backend) with zero new warnings, except the 2 pre-existing `react-refresh/only-export-components` warnings already present in `frontend/src/routes/router.tsx` (do not add more of that same warning to *new* files — keep hooks and components in separate files there).
- Money/None: no currency handling in this phase — irrelevant, skip.
- Card `position` in `PATCH /cards/{id}/move` is a 0-based insertion index into the **target column's** card list after the move (see `backend/internal/card/service.go`'s `MoveCard`/`reorderWithInsert`) — not an arbitrary integer requiring gap math.
- `PATCH /boards/{boardID}/columns/reorder` and the card move endpoint both return based on a **full replace** semantics for order (reorder takes the complete new ordering of column IDs; move recomputes the full target-column order server-side) — the frontend must send complete orderings, not deltas.

---

## Task 1: Backend — WebSocket Origin hardening

**Files:**
- Modify: `backend/internal/realtime/handler.go`
- Modify: `backend/cmd/api/main.go`
- Modify: `backend/internal/realtime/handler_test.go`
- Modify: `backend/internal/realtime/e2e_test.go`

**Interfaces:**
- Consumes: `cfg.CORSAllowedOrigin` (already loaded by `config.Load()`, already used for the HTTP CORS middleware in `main.go:64`).
- Produces: `realtime.NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer, allowedOrigin string) *Handler` — the new fourth parameter. No other package depends on `realtime.NewHandler`'s signature, so this is the only call-site ripple.

- [ ] **Step 1: Update `Handler` and `NewHandler` to take the allowed origin**

In `backend/internal/realtime/handler.go`, change:

```go
type Handler struct {
	hub    *Hub
	tokens TokenParser
	board  BoardAuthorizer
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board}
}
```

to:

```go
type Handler struct {
	hub           *Hub
	tokens        TokenParser
	board         BoardAuthorizer
	allowedOrigin string
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer, allowedOrigin string) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board, allowedOrigin: allowedOrigin}
}
```

- [ ] **Step 2: Replace `InsecureSkipVerify` with `OriginPatterns` in `ServeWS`**

Replace this block in `ServeWS`:

```go
	// InsecureSkipVerify disables the Origin check. Acceptable for now:
	// there is no browser frontend yet (Phase 5+) and no cookie-based
	// auth on this endpoint (the token is an explicit query parameter,
	// not an ambient credential), so there's no CSRF-style risk this
	// check would prevent. Revisit once the frontend's origin is known
	// and replace with an explicit OriginPatterns allowlist.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
```

with:

```go
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{h.allowedOrigin}})
```

`coder/websocket`'s `authenticateOrigin` (see `accept.go`) only enforces the check when the request actually carries an `Origin` header — requests without one (curl, server-to-server, the existing tests) are unaffected. Browsers always send `Origin` on a WebSocket handshake, so this closes the gap the removed comment described.

- [ ] **Step 3: Wire the new parameter in `main.go`**

In `backend/cmd/api/main.go`, change:

```go
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService)
```

to:

```go
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService, cfg.CORSAllowedOrigin)
```

- [ ] **Step 4: Update existing test call sites**

In `backend/internal/realtime/handler_test.go`, update all three `NewHandler(...)` calls to pass an allowed origin, e.g.:

```go
h := NewHandler(hub, &fakeTokenParser{userID: uuid.New()}, &fakeWSBoardAuthorizer{allow: true}, "http://localhost:5173")
```

(same literal `"http://localhost:5173"` for all three existing calls — matches `config.Load()`'s default `CORS_ALLOWED_ORIGIN`).

In `backend/internal/realtime/e2e_test.go`, update:

```go
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService)
```

to:

```go
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService, "http://localhost:5173")
```

- [ ] **Step 5: Add a test for a rejected cross-origin handshake**

Add to `backend/internal/realtime/handler_test.go`:

```go
func TestHandler_ServeWS_RejectsDisallowedOrigin(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	boardID := uuid.New()
	h := NewHandler(hub, &fakeTokenParser{userID: userID}, &fakeWSBoardAuthorizer{allow: true}, "http://localhost:5173")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/boards/" + boardID.String() + "/ws?token=whatever"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example.com"}},
	})
	require.Error(t, err)
}
```

Run `go test ./internal/realtime/...` and confirm all tests (including the new one and the three pre-existing ones) pass.

- [ ] **Step 6: Run the full backend suite and commit**

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

All green, then:

```bash
git add internal/realtime/handler.go internal/realtime/handler_test.go internal/realtime/e2e_test.go cmd/api/main.go
git commit -m "fix(backend): enforce WebSocket Origin check now that the frontend origin is known"
```

---

## Task 2: Backend — board members list includes name and email

**Files:**
- Modify: `backend/db/queries/board_members.sql`
- Modify: `backend/internal/board/domain.go`
- Modify: `backend/internal/board/repository_postgres.go`
- Modify: `backend/internal/board/handler.go`
- Modify: `backend/internal/board/repository_postgres_test.go`
- Regenerate: `backend/internal/platform/db/gen/board_members.sql.go` (via `sqlc generate`)

**Interfaces:**
- Consumes: `users` table (`id`, `name`, `email`) already defined in `backend/db/migrations`.
- Produces: `board.Member` struct gains `Name string` and `Email string` fields, populated only by `ListMembers`/`GET /boards/{boardID}/members` (the only place they're needed). `GetMember`, `AddMember`, `RemoveMember` are untouched and continue returning/accepting `Member` without those fields populated on read (zero value) — this is fine, nothing reads `Name`/`Email` from their results. The `memberView` JSON response gains `"name"` and `"email"` fields.

- [ ] **Step 1: Change the `ListBoardMembers` query to join `users`**

In `backend/db/queries/board_members.sql`, replace:

```sql
-- name: ListBoardMembers :many
SELECT * FROM board_members WHERE board_id = $1
ORDER BY created_at ASC;
```

with:

```sql
-- name: ListBoardMembers :many
SELECT bm.board_id, bm.user_id, bm.role, bm.created_at, u.name, u.email
FROM board_members bm
JOIN users u ON u.id = bm.user_id
WHERE bm.board_id = $1
ORDER BY bm.created_at ASC;
```

Leave `AddBoardMember`, `RemoveBoardMember`, and `GetBoardMember` in that file unchanged.

- [ ] **Step 2: Regenerate sqlc code**

From `backend/`:

```bash
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate
```

This changes `ListBoardMembers`'s generated return type from `[]BoardMember` to `[]ListBoardMembersRow` (a new struct with `BoardID`, `UserID`, `Role`, `CreatedAt`, `Name`, `Email` fields) in `backend/internal/platform/db/gen/board_members.sql.go`. Confirm the diff looks like that — do not hand-edit the generated file.

- [ ] **Step 3: Add `Name`/`Email` to the domain `Member` struct**

In `backend/internal/board/domain.go`, change:

```go
type Member struct {
	BoardID   uuid.UUID
	UserID    uuid.UUID
	Role      Role
	CreatedAt time.Time
}
```

to:

```go
type Member struct {
	BoardID   uuid.UUID
	UserID    uuid.UUID
	Role      Role
	Name      string
	Email     string
	CreatedAt time.Time
}
```

- [ ] **Step 4: Update the repository mapping**

In `backend/internal/board/repository_postgres.go`, `ListMembers` currently reuses `toDomainMember(row gen.BoardMember)`, shared with `GetMember`. Since the query's row type changed, give `ListMembers` its own mapping function instead of touching `toDomainMember` (which `GetMember` still needs, unchanged, for `gen.BoardMember`):

```go
func (r *PostgresRepository) ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error) {
	rows, err := r.q.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, toDomainMemberWithProfile(row))
	}
	return members, nil
}

func toDomainMemberWithProfile(row gen.ListBoardMembersRow) Member {
	return Member{
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      Role(row.Role),
		Name:      row.Name,
		Email:     row.Email,
		CreatedAt: row.CreatedAt,
	}
}
```

Leave `toDomainMember(row gen.BoardMember)` exactly as-is; `GetMember` keeps calling it.

- [ ] **Step 5: Expose the fields in the HTTP response**

In `backend/internal/board/handler.go`, change:

```go
type memberView struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}
```

to:

```go
type memberView struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
```

and `toMemberView`:

```go
func toMemberView(m Member) memberView {
	return memberView{
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		Name:      m.Name,
		Email:     m.Email,
		CreatedAt: m.CreatedAt,
	}
}
```

Note: `InviteMember`'s response also goes through `toMemberView`, but the `Member` it builds in `service.go`'s `InviteMember` (`Member{BoardID: boardID, UserID: userID, Role: RoleMember}`) doesn't set `Name`/`Email`, so the invite response will have empty `name`/`email` strings. That's acceptable for this task — the members panel (Task 8) refetches the list after inviting, which does carry them. Do not modify `service.go`'s `InviteMember` to look up the profile; that's out of scope here.

- [ ] **Step 6: Update the integration test**

In `backend/internal/board/repository_postgres_test.go`, `TestPostgresRepository_MemberLifecycle` calls `createTestUser(t, ctx, q, "member2@example.com")` and only asserts `require.Len(t, members, 2)`. Extend it to assert the joined fields are populated, e.g. add after the existing `require.Len` line:

```go
	for _, m := range members {
		require.NotEmpty(t, m.Email)
		require.NotEmpty(t, m.Name)
	}
```

This test is build-tagged `integration` and is not part of the default `go test ./...` run — you do not need a live Postgres to complete this task, but the edit must be syntactically correct Go (it will be compiled, just not executed, by the default build).

- [ ] **Step 7: Run the full backend suite and commit**

```bash
cd backend
go build ./...
go vet ./...
go test ./...
```

All green (the changed integration test file must still compile — `go build ./...` covers that even though it won't run). Then:

```bash
git add db/queries/board_members.sql internal/platform/db/gen/board_members.sql.go internal/board/domain.go internal/board/repository_postgres.go internal/board/handler.go internal/board/repository_postgres_test.go
git commit -m "feat(backend): include member name and email in the members list response"
```

---

## Task 3: Frontend — dependencies, React Query wiring, and API modules

**Files:**
- Modify: `frontend/package.json` (add dependencies)
- Create: `frontend/src/lib/queryClient.ts`
- Create: `frontend/src/lib/queryKeys.ts`
- Modify: `frontend/src/main.tsx`
- Create: `frontend/src/api/cards.ts`
- Create: `frontend/src/api/cards.test.ts`
- Create: `frontend/src/api/columns.ts`
- Create: `frontend/src/api/columns.test.ts`
- Create: `frontend/src/api/boards.ts`
- Create: `frontend/src/api/boards.test.ts`

**Interfaces:**
- Consumes: `apiFetch` from `frontend/src/api/client.ts` (unchanged, from Phase 5).
- Produces (consumed by every later task in this plan):
  - `queryClient` (default `QueryClient` instance) from `frontend/src/lib/queryClient.ts`.
  - `boardKeys` from `frontend/src/lib/queryKeys.ts`: `{ all, list(), detail(boardId), columns(boardId), members(boardId) }`.
  - `frontend/src/api/boards.ts`: `Board { id, name, ownerId, createdAt, updatedAt }`, `Member { userId, role: 'owner' | 'member', name, email, createdAt }`, functions `listBoards()`, `createBoard(name)`, `renameBoard(boardId, name)`, `deleteBoard(boardId)`, `listMembers(boardId)`, `inviteMember(boardId, email)`, `removeMember(boardId, userId)`.
  - `frontend/src/api/columns.ts`: `Column { id, boardId, title, position, createdAt, updatedAt }`, `ColumnWithCards extends Column { cards: Card[] }`, functions `listColumns(boardId)`, `createColumn(boardId, title)`, `renameColumn(boardId, columnId, title)`, `deleteColumn(boardId, columnId)`, `reorderColumns(boardId, columnIds: string[])`.
  - `frontend/src/api/cards.ts`: `Card { id, columnId, title, description, position, assigneeId: string | null, dueDate: string | null, createdAt, updatedAt }`, functions `createCard(columnId, title, description?)`, `updateCard(cardId, title, description, assigneeId, dueDate)`, `deleteCard(cardId)`, `moveCard(cardId, columnId, position)`.

- [ ] **Step 1: Install dependencies**

```bash
cd frontend
npm install @tanstack/react-query @dnd-kit/core @dnd-kit/sortable @dnd-kit/utilities
```

This adds `@tanstack/react-query`, `@dnd-kit/core`, `@dnd-kit/sortable`, and `@dnd-kit/utilities` to `dependencies` in `package.json` (and updates `package-lock.json`). No dev dependencies are needed for this task.

- [ ] **Step 2: Create the query client**

`frontend/src/lib/queryClient.ts`:

```ts
import { QueryClient } from '@tanstack/react-query'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})
```

- [ ] **Step 3: Create the query key helpers**

`frontend/src/lib/queryKeys.ts`:

```ts
export const boardKeys = {
  all: ['boards'] as const,
  list: () => [...boardKeys.all, 'list'] as const,
  detail: (boardId: string) => [...boardKeys.all, boardId] as const,
  columns: (boardId: string) => [...boardKeys.all, boardId, 'columns'] as const,
  members: (boardId: string) => [...boardKeys.all, boardId, 'members'] as const,
}
```

- [ ] **Step 4: Wrap the app in `QueryClientProvider`**

In `frontend/src/main.tsx`, change:

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

to:

```tsx
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.tsx'
import { queryClient } from './lib/queryClient'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
)
```

- [ ] **Step 5: Write the failing tests for `api/cards.ts`**

`frontend/src/api/cards.test.ts`:

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { createCard, updateCard, deleteCard, moveCard } from './cards'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const cardBody = {
  id: 'card-1',
  column_id: 'col-1',
  title: 'Write tests',
  description: 'Cover the happy path',
  position: 0,
  assignee_id: null,
  due_date: null,
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
}

describe('cards API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('createCard posts to /cards with column_id, title, and description', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    const result = await createCard('col-1', 'Write tests', 'Cover the happy path')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards', {
      method: 'POST',
      body: { column_id: 'col-1', title: 'Write tests', description: 'Cover the happy path' },
    })
    expect(result).toEqual({
      id: 'card-1',
      columnId: 'col-1',
      title: 'Write tests',
      description: 'Cover the happy path',
      position: 0,
      assigneeId: null,
      dueDate: null,
      createdAt: '2026-08-12T00:00:00Z',
      updatedAt: '2026-08-12T00:00:00Z',
    })
  })

  it('createCard defaults description to an empty string', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await createCard('col-1', 'Write tests')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards', {
      method: 'POST',
      body: { column_id: 'col-1', title: 'Write tests', description: '' },
    })
  })

  it('updateCard patches /cards/{id} with the full editable field set', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await updateCard('card-1', 'New title', 'New description', 'user-2', '2026-09-01T00:00:00Z')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1', {
      method: 'PATCH',
      body: {
        title: 'New title',
        description: 'New description',
        assignee_id: 'user-2',
        due_date: '2026-09-01T00:00:00Z',
      },
    })
  })

  it('deleteCard deletes /cards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteCard('card-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1', { method: 'DELETE' })
  })

  it('moveCard patches /cards/{id}/move with the target column and position', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(cardBody)

    await moveCard('card-1', 'col-2', 3)

    expect(client.apiFetch).toHaveBeenCalledWith('/cards/card-1/move', {
      method: 'PATCH',
      body: { column_id: 'col-2', position: 3 },
    })
  })
})
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `npm test -- --run src/api/cards.test.ts`
Expected: FAIL — `./cards` module not found.

- [ ] **Step 7: Implement `api/cards.ts`**

`frontend/src/api/cards.ts`:

```ts
import { apiFetch } from './client'

export interface Card {
  id: string
  columnId: string
  title: string
  description: string
  position: number
  assigneeId: string | null
  dueDate: string | null
  createdAt: string
  updatedAt: string
}

export interface CardBody {
  id: string
  column_id: string
  title: string
  description: string
  position: number
  assignee_id: string | null
  due_date: string | null
  created_at: string
  updated_at: string
}

export function toCard(body: CardBody): Card {
  return {
    id: body.id,
    columnId: body.column_id,
    title: body.title,
    description: body.description,
    position: body.position,
    assigneeId: body.assignee_id,
    dueDate: body.due_date,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

export async function createCard(columnId: string, title: string, description = ''): Promise<Card> {
  const body = await apiFetch<CardBody>('/cards', {
    method: 'POST',
    body: { column_id: columnId, title, description },
  })
  return toCard(body)
}

export async function updateCard(
  cardId: string,
  title: string,
  description: string,
  assigneeId: string | null,
  dueDate: string | null,
): Promise<Card> {
  const body = await apiFetch<CardBody>(`/cards/${cardId}`, {
    method: 'PATCH',
    body: { title, description, assignee_id: assigneeId, due_date: dueDate },
  })
  return toCard(body)
}

export async function deleteCard(cardId: string): Promise<void> {
  await apiFetch<void>(`/cards/${cardId}`, { method: 'DELETE' })
}

export async function moveCard(cardId: string, columnId: string, position: number): Promise<Card> {
  const body = await apiFetch<CardBody>(`/cards/${cardId}/move`, {
    method: 'PATCH',
    body: { column_id: columnId, position },
  })
  return toCard(body)
}
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `npm test -- --run src/api/cards.test.ts`
Expected: PASS

- [ ] **Step 9: Write the failing tests for `api/columns.ts`**

`frontend/src/api/columns.test.ts`:

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { listColumns, createColumn, renameColumn, deleteColumn, reorderColumns } from './columns'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const columnBody = {
  id: 'col-1',
  board_id: 'board-1',
  title: 'To do',
  position: 0,
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
  cards: [
    {
      id: 'card-1',
      column_id: 'col-1',
      title: 'Write tests',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '2026-08-12T00:00:00Z',
      updated_at: '2026-08-12T00:00:00Z',
    },
  ],
}

describe('columns API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('listColumns fetches /boards/{id}/columns and maps embedded cards', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([columnBody])

    const result = await listColumns('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns')
    expect(result).toEqual([
      {
        id: 'col-1',
        boardId: 'board-1',
        title: 'To do',
        position: 0,
        createdAt: '2026-08-12T00:00:00Z',
        updatedAt: '2026-08-12T00:00:00Z',
        cards: [
          {
            id: 'card-1',
            columnId: 'col-1',
            title: 'Write tests',
            description: '',
            position: 0,
            assigneeId: null,
            dueDate: null,
            createdAt: '2026-08-12T00:00:00Z',
            updatedAt: '2026-08-12T00:00:00Z',
          },
        ],
      },
    ])
  })

  it('listColumns defaults cards to an empty array when omitted', async () => {
    const { cards: _cards, ...withoutCards } = columnBody
    vi.mocked(client.apiFetch).mockResolvedValue([withoutCards])

    const result = await listColumns('board-1')

    expect(result[0].cards).toEqual([])
  })

  it('createColumn posts to /boards/{id}/columns with the title', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(columnBody)

    await createColumn('board-1', 'To do')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns', {
      method: 'POST',
      body: { title: 'To do' },
    })
  })

  it('renameColumn patches /boards/{id}/columns/{columnId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(columnBody)

    await renameColumn('board-1', 'col-1', 'Doing')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/col-1', {
      method: 'PATCH',
      body: { title: 'Doing' },
    })
  })

  it('deleteColumn deletes /boards/{id}/columns/{columnId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteColumn('board-1', 'col-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/col-1', { method: 'DELETE' })
  })

  it('reorderColumns patches /boards/{id}/columns/reorder with the full ordering', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await reorderColumns('board-1', ['col-2', 'col-1'])

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/columns/reorder', {
      method: 'PATCH',
      body: { column_ids: ['col-2', 'col-1'] },
    })
  })
})
```

- [ ] **Step 10: Run the test to verify it fails**

Run: `npm test -- --run src/api/columns.test.ts`
Expected: FAIL — `./columns` module not found.

- [ ] **Step 11: Implement `api/columns.ts`**

`frontend/src/api/columns.ts`:

```ts
import { apiFetch } from './client'
import { toCard, type Card, type CardBody } from './cards'

export interface Column {
  id: string
  boardId: string
  title: string
  position: number
  createdAt: string
  updatedAt: string
}

export interface ColumnWithCards extends Column {
  cards: Card[]
}

interface ColumnBody {
  id: string
  board_id: string
  title: string
  position: number
  created_at: string
  updated_at: string
  cards?: CardBody[]
}

function toColumn(body: ColumnBody): Column {
  return {
    id: body.id,
    boardId: body.board_id,
    title: body.title,
    position: body.position,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

function toColumnWithCards(body: ColumnBody): ColumnWithCards {
  return { ...toColumn(body), cards: (body.cards ?? []).map(toCard) }
}

export async function listColumns(boardId: string): Promise<ColumnWithCards[]> {
  const body = await apiFetch<ColumnBody[]>(`/boards/${boardId}/columns`)
  return body.map(toColumnWithCards)
}

export async function createColumn(boardId: string, title: string): Promise<Column> {
  const body = await apiFetch<ColumnBody>(`/boards/${boardId}/columns`, {
    method: 'POST',
    body: { title },
  })
  return toColumn(body)
}

export async function renameColumn(boardId: string, columnId: string, title: string): Promise<Column> {
  const body = await apiFetch<ColumnBody>(`/boards/${boardId}/columns/${columnId}`, {
    method: 'PATCH',
    body: { title },
  })
  return toColumn(body)
}

export async function deleteColumn(boardId: string, columnId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/columns/${columnId}`, { method: 'DELETE' })
}

export async function reorderColumns(boardId: string, columnIds: string[]): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/columns/reorder`, {
    method: 'PATCH',
    body: { column_ids: columnIds },
  })
}
```

- [ ] **Step 12: Run the test to verify it passes**

Run: `npm test -- --run src/api/columns.test.ts`
Expected: PASS

- [ ] **Step 13: Write the failing tests for `api/boards.ts`**

`frontend/src/api/boards.test.ts`:

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { listBoards, createBoard, renameBoard, deleteBoard, listMembers, inviteMember, removeMember } from './boards'
import * as client from './client'

vi.mock('./client', async () => {
  const actual = await vi.importActual<typeof import('./client')>('./client')
  return { ...actual, apiFetch: vi.fn() }
})

const boardBody = {
  id: 'board-1',
  name: 'Sprint Board',
  owner_id: 'user-1',
  created_at: '2026-08-12T00:00:00Z',
  updated_at: '2026-08-12T00:00:00Z',
}

const memberBody = {
  user_id: 'user-2',
  role: 'member',
  name: 'Ada Lovelace',
  email: 'ada@example.com',
  created_at: '2026-08-12T00:00:00Z',
}

describe('boards API', () => {
  beforeEach(() => {
    vi.mocked(client.apiFetch).mockReset()
  })

  it('listBoards fetches /boards and maps the response', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([boardBody])

    const result = await listBoards()

    expect(client.apiFetch).toHaveBeenCalledWith('/boards')
    expect(result).toEqual([
      { id: 'board-1', name: 'Sprint Board', ownerId: 'user-1', createdAt: '2026-08-12T00:00:00Z', updatedAt: '2026-08-12T00:00:00Z' },
    ])
  })

  it('createBoard posts to /boards with the name', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(boardBody)

    await createBoard('Sprint Board')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards', { method: 'POST', body: { name: 'Sprint Board' } })
  })

  it('renameBoard patches /boards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(boardBody)

    await renameBoard('board-1', 'New name')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1', { method: 'PATCH', body: { name: 'New name' } })
  })

  it('deleteBoard deletes /boards/{id}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await deleteBoard('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1', { method: 'DELETE' })
  })

  it('listMembers fetches /boards/{id}/members and maps the response', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue([memberBody])

    const result = await listMembers('board-1')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members')
    expect(result).toEqual([
      { userId: 'user-2', role: 'member', name: 'Ada Lovelace', email: 'ada@example.com', createdAt: '2026-08-12T00:00:00Z' },
    ])
  })

  it('inviteMember posts to /boards/{id}/members with the email', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(memberBody)

    await inviteMember('board-1', 'ada@example.com')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members', {
      method: 'POST',
      body: { email: 'ada@example.com' },
    })
  })

  it('removeMember deletes /boards/{id}/members/{userId}', async () => {
    vi.mocked(client.apiFetch).mockResolvedValue(undefined)

    await removeMember('board-1', 'user-2')

    expect(client.apiFetch).toHaveBeenCalledWith('/boards/board-1/members/user-2', { method: 'DELETE' })
  })
})
```

- [ ] **Step 14: Run the test to verify it fails**

Run: `npm test -- --run src/api/boards.test.ts`
Expected: FAIL — `./boards` module not found.

- [ ] **Step 15: Implement `api/boards.ts`**

`frontend/src/api/boards.ts`:

```ts
import { apiFetch } from './client'

export interface Board {
  id: string
  name: string
  ownerId: string
  createdAt: string
  updatedAt: string
}

export interface Member {
  userId: string
  role: 'owner' | 'member'
  name: string
  email: string
  createdAt: string
}

interface BoardBody {
  id: string
  name: string
  owner_id: string
  created_at: string
  updated_at: string
}

interface MemberBody {
  user_id: string
  role: string
  name: string
  email: string
  created_at: string
}

function toBoard(body: BoardBody): Board {
  return {
    id: body.id,
    name: body.name,
    ownerId: body.owner_id,
    createdAt: body.created_at,
    updatedAt: body.updated_at,
  }
}

function toMember(body: MemberBody): Member {
  return {
    userId: body.user_id,
    role: body.role as Member['role'],
    name: body.name,
    email: body.email,
    createdAt: body.created_at,
  }
}

export async function listBoards(): Promise<Board[]> {
  const body = await apiFetch<BoardBody[]>('/boards')
  return body.map(toBoard)
}

export async function createBoard(name: string): Promise<Board> {
  const body = await apiFetch<BoardBody>('/boards', { method: 'POST', body: { name } })
  return toBoard(body)
}

export async function renameBoard(boardId: string, name: string): Promise<Board> {
  const body = await apiFetch<BoardBody>(`/boards/${boardId}`, { method: 'PATCH', body: { name } })
  return toBoard(body)
}

export async function deleteBoard(boardId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}`, { method: 'DELETE' })
}

export async function listMembers(boardId: string): Promise<Member[]> {
  const body = await apiFetch<MemberBody[]>(`/boards/${boardId}/members`)
  return body.map(toMember)
}

export async function inviteMember(boardId: string, email: string): Promise<Member> {
  const body = await apiFetch<MemberBody>(`/boards/${boardId}/members`, {
    method: 'POST',
    body: { email },
  })
  return toMember(body)
}

export async function removeMember(boardId: string, userId: string): Promise<void> {
  await apiFetch<void>(`/boards/${boardId}/members/${userId}`, { method: 'DELETE' })
}
```

- [ ] **Step 16: Run the test to verify it passes**

Run: `npm test -- --run src/api/boards.test.ts`
Expected: PASS

- [ ] **Step 17: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green (36 pre-existing tests + the new ones from this task, 0 lint errors), then:

```bash
git add package.json package-lock.json src/lib/queryClient.ts src/lib/queryKeys.ts src/main.tsx src/api/boards.ts src/api/boards.test.ts src/api/columns.ts src/api/columns.test.ts src/api/cards.ts src/api/cards.test.ts
git commit -m "feat(frontend): add React Query wiring and boards/columns/cards API modules"
```

---

## Task 4: Frontend — board list page

**Files:**
- Create: `frontend/src/components/ui/Modal.tsx`
- Create: `frontend/src/components/ui/Modal.test.tsx`
- Create: `frontend/src/features/boards/BoardListPage.tsx`
- Create: `frontend/src/features/boards/BoardListPage.test.tsx`
- Modify: `frontend/src/routes/router.tsx`

**Interfaces:**
- Consumes: `boardKeys`, `listBoards`, `createBoard` from Task 3.
- Produces: `Modal` (`frontend/src/components/ui/Modal.tsx`, default export, props `{ title: string; onClose: () => void; children: React.ReactNode }`) — reused by Tasks 5 and 8. `BoardListPage` (default export) mounted at route `/`.

- [ ] **Step 1: Write the failing test for `Modal`**

`frontend/src/components/ui/Modal.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Modal from './Modal'

describe('Modal', () => {
  it('renders the title and children', () => {
    render(
      <Modal title="New board" onClose={vi.fn()}>
        <p>Form goes here</p>
      </Modal>,
    )

    expect(screen.getByText('New board')).toBeInTheDocument()
    expect(screen.getByText('Form goes here')).toBeInTheDocument()
  })

  it('calls onClose when the close button is clicked', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.click(screen.getByRole('button', { name: /fechar/i }))

    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when the backdrop is clicked', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.click(screen.getByTestId('modal-backdrop'))

    expect(onClose).toHaveBeenCalled()
  })

  it('calls onClose when Escape is pressed', async () => {
    const onClose = vi.fn()
    render(
      <Modal title="New board" onClose={onClose}>
        <p>Form</p>
      </Modal>,
    )

    await userEvent.keyboard('{Escape}')

    expect(onClose).toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm test -- --run src/components/ui/Modal.test.tsx`
Expected: FAIL — `./Modal` module not found.

- [ ] **Step 3: Implement `Modal`**

`frontend/src/components/ui/Modal.tsx`:

```tsx
import { useEffect } from 'react'
import type { ReactNode } from 'react'

interface ModalProps {
  title: string
  onClose: () => void
  children: ReactNode
}

export default function Modal({ title, onClose, children }: ModalProps) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        onClose()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  return (
    <div
      data-testid="modal-backdrop"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      onClick={onClose}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Fechar"
            className="text-gray-400 hover:text-gray-600"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm test -- --run src/components/ui/Modal.test.tsx`
Expected: PASS

- [ ] **Step 5: Write the failing tests for `BoardListPage`**

`frontend/src/features/boards/BoardListPage.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BoardListPage from './BoardListPage'
import * as boardsApi from '../../api/boards'

vi.mock('../../api/boards')

function renderWithProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <BoardListPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const board = {
  id: 'board-1',
  name: 'Sprint Board',
  ownerId: 'user-1',
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

describe('BoardListPage', () => {
  beforeEach(() => {
    vi.mocked(boardsApi.listBoards).mockReset()
    vi.mocked(boardsApi.createBoard).mockReset()
  })

  it('renders the fetched boards', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValue([board])
    renderWithProviders()

    expect(await screen.findByText('Sprint Board')).toBeInTheDocument()
  })

  it('shows an empty state when there are no boards', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValue([])
    renderWithProviders()

    expect(await screen.findByText(/nenhum board ainda/i)).toBeInTheDocument()
  })

  it('shows a retry option when the fetch fails', async () => {
    vi.mocked(boardsApi.listBoards).mockRejectedValue(new Error('network down'))
    renderWithProviders()

    expect(await screen.findByRole('button', { name: /tentar novamente/i })).toBeInTheDocument()
  })

  it('creates a board and adds it to the list', async () => {
    vi.mocked(boardsApi.listBoards).mockResolvedValueOnce([]).mockResolvedValueOnce([board])
    vi.mocked(boardsApi.createBoard).mockResolvedValue(board)
    renderWithProviders()

    await screen.findByText(/nenhum board ainda/i)

    await userEvent.click(screen.getByRole('button', { name: /novo board/i }))
    await userEvent.type(screen.getByLabelText(/nome/i), 'Sprint Board')
    await userEvent.click(screen.getByRole('button', { name: /criar/i }))

    await waitFor(() => expect(boardsApi.createBoard).toHaveBeenCalledWith('Sprint Board'))
    expect(await screen.findByText('Sprint Board')).toBeInTheDocument()
  })
})
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `npm test -- --run src/features/boards/BoardListPage.test.tsx`
Expected: FAIL — `./BoardListPage` module not found.

- [ ] **Step 7: Implement `BoardListPage`**

`frontend/src/features/boards/BoardListPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listBoards, createBoard } from '../../api/boards'
import Modal from '../../components/ui/Modal'

export default function BoardListPage() {
  const queryClient = useQueryClient()
  const [isCreating, setIsCreating] = useState(false)
  const [name, setName] = useState('')

  const { data: boards, isPending, isError, refetch } = useQuery({
    queryKey: boardKeys.list(),
    queryFn: listBoards,
  })

  const createMutation = useMutation({
    mutationFn: (boardName: string) => createBoard(boardName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.list() })
      setIsCreating(false)
      setName('')
    },
  })

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createMutation.mutate(name)
  }

  if (isPending) {
    return (
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-24 animate-pulse rounded-lg bg-gray-200" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-red-700">Não foi possível carregar os boards.</p>
        <button
          type="button"
          onClick={() => refetch()}
          className="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-100"
        >
          Tentar novamente
        </button>
      </div>
    )
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Seus boards</h1>
        <button
          type="button"
          onClick={() => setIsCreating(true)}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-700"
        >
          Novo board
        </button>
      </div>

      {boards.length === 0 ? (
        <p className="text-sm text-gray-600">Nenhum board ainda — crie o primeiro.</p>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {boards.map((board) => (
            <Link
              key={board.id}
              to={`/boards/${board.id}`}
              className="rounded-lg border border-gray-200 bg-white p-4 shadow-sm hover:border-blue-400 hover:shadow"
            >
              <h2 className="font-medium text-gray-900">{board.name}</h2>
            </Link>
          ))}
        </div>
      )}

      {isCreating && (
        <Modal title="Novo board" onClose={() => setIsCreating(false)}>
          <form onSubmit={handleSubmit} className="space-y-4">
            {createMutation.isError && (
              <p className="rounded bg-red-50 p-2 text-sm text-red-700">Não foi possível criar o board.</p>
            )}
            <div>
              <label htmlFor="board-name" className="block text-sm font-medium text-gray-700">
                Nome
              </label>
              <input
                id="board-name"
                type="text"
                required
                value={name}
                onChange={(event) => setName(event.target.value)}
                className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
              />
            </div>
            <button
              type="submit"
              disabled={createMutation.isPending}
              className="w-full rounded bg-blue-600 py-2 text-white disabled:opacity-50"
            >
              Criar
            </button>
          </form>
        </Modal>
      )}
    </div>
  )
}
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `npm test -- --run src/features/boards/BoardListPage.test.tsx`
Expected: PASS

- [ ] **Step 9: Wire the route and remove the placeholder `HomePage`**

In `frontend/src/routes/router.tsx`, remove the `HomePage` function entirely and its import isn't needed (it was defined inline, not imported), then change:

```tsx
import RequireAuth from './RequireAuth'
import AppLayout from '../components/layout/AppLayout'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import { useAuthStore } from '../features/auth/useAuthStore'

function HomePage() {
  const user = useAuthStore((state) => state.user)
  return <p className="text-gray-700">Bem-vindo, {user?.name}. A lista de boards chega na próxima fase.</p>
}
```

to:

```tsx
import RequireAuth from './RequireAuth'
import AppLayout from '../components/layout/AppLayout'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import BoardListPage from '../features/boards/BoardListPage'
import { useAuthStore } from '../features/auth/useAuthStore'
```

and change the routes array's home entry:

```tsx
        children: [{ path: '/', element: <HomePage /> }],
```

to:

```tsx
        children: [{ path: '/', element: <BoardListPage /> }],
```

`useAuthStore` stays imported — `RedirectIfAuthenticated` still uses it.

- [ ] **Step 10: Update `App.test.tsx` if it asserted on the placeholder text**

Check `frontend/src/App.test.tsx` for any assertion on the old "A lista de boards chega na próxima fase" or "Bem-vindo" text. If present, it will now fail because `BoardListPage` fetches via `useQuery` with no `QueryClientProvider` wrapping `App` in that test — wrap the `render(<App />)` calls in that file with `QueryClientProvider` using a fresh `QueryClient({ defaultOptions: { queries: { retry: false } } })`, and update any assertion on the old placeholder text to something resilient to the loading/empty state instead (or mock `../api/boards` there too, matching the pattern in Step 5). Read the current file before deciding which is needed — do not guess blind.

- [ ] **Step 11: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green, 0 new lint warnings, then:

```bash
git add src/components/ui/Modal.tsx src/components/ui/Modal.test.tsx src/features/boards/BoardListPage.tsx src/features/boards/BoardListPage.test.tsx src/routes/router.tsx src/App.test.tsx
git commit -m "feat(frontend): add board list page with create-board flow"
```

---

## Task 5: Frontend — kanban board view (columns, cards, CRUD, no drag yet)

**Files:**
- Create: `frontend/src/features/board/BoardPage.tsx`
- Create: `frontend/src/features/board/BoardPage.test.tsx`
- Create: `frontend/src/features/board/Column.tsx`
- Create: `frontend/src/features/board/Column.test.tsx`
- Create: `frontend/src/features/board/CardItem.tsx`
- Create: `frontend/src/features/board/CardItem.test.tsx`
- Create: `frontend/src/features/board/CardDetailModal.tsx`
- Create: `frontend/src/features/board/CardDetailModal.test.tsx`
- Modify: `frontend/src/routes/router.tsx`

**Interfaces:**
- Consumes: `boardKeys`, `listColumns`/`createColumn`/`renameColumn`/`deleteColumn` (Task 3), `createCard`/`updateCard`/`deleteCard` (Task 3), `Modal` (Task 4).
- Produces: `BoardPage` (default export) mounted at `/boards/:boardId`. `Column` (default export, props `{ column: ColumnWithCards; boardId: string }`). `CardItem` (default export, props `{ card: Card; onClick: () => void }`). `CardDetailModal` (default export, props `{ card: Card; onClose: () => void }`). Tasks 6 and 7 both modify `BoardPage.tsx`; Task 7 also modifies `Column.tsx` and `CardItem.tsx`.

- [ ] **Step 1: Write the failing test for `CardItem`**

`frontend/src/features/board/CardItem.test.tsx`:

```tsx
import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import CardItem from './CardItem'
import type { Card } from '../../api/cards'

const card: Card = {
  id: 'card-1',
  columnId: 'col-1',
  title: 'Write tests',
  description: 'Cover the happy path and edge cases in detail',
  position: 0,
  assigneeId: null,
  dueDate: null,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

describe('CardItem', () => {
  it('renders the title and a truncated description', () => {
    render(<CardItem card={card} onClick={vi.fn()} />)

    expect(screen.getByText('Write tests')).toBeInTheDocument()
    expect(screen.getByText(/cover the happy path/i)).toBeInTheDocument()
  })

  it('calls onClick when clicked', async () => {
    const onClick = vi.fn()
    render(<CardItem card={card} onClick={onClick} />)

    await userEvent.click(screen.getByText('Write tests'))

    expect(onClick).toHaveBeenCalled()
  })

  it('shows a due date badge when the card has one', () => {
    render(<CardItem card={{ ...card, dueDate: '2026-09-01T00:00:00Z' }} onClick={vi.fn()} />)

    expect(screen.getByText(/2026-09-01/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/CardItem.test.tsx`
Expected: FAIL — `./CardItem` module not found.

- [ ] **Step 3: Implement `CardItem`**

`frontend/src/features/board/CardItem.tsx`:

```tsx
import type { Card } from '../../api/cards'

interface CardItemProps {
  card: Card
  onClick: () => void
}

export default function CardItem({ card, onClick }: CardItemProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="w-full rounded border border-gray-200 bg-white p-3 text-left shadow-sm hover:border-blue-400"
    >
      <p className="font-medium text-gray-900">{card.title}</p>
      {card.description && (
        <p className="mt-1 line-clamp-2 text-sm text-gray-600">{card.description}</p>
      )}
      {card.dueDate && (
        <span className="mt-2 inline-block rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
          {card.dueDate.slice(0, 10)}
        </span>
      )}
    </button>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/CardItem.test.tsx`
Expected: PASS

- [ ] **Step 5: Write the failing test for `CardDetailModal`**

`frontend/src/features/board/CardDetailModal.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import CardDetailModal from './CardDetailModal'
import * as cardsApi from '../../api/cards'
import type { Card } from '../../api/cards'

vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, updateCard: vi.fn(), deleteCard: vi.fn() }
})

const card: Card = {
  id: 'card-1',
  columnId: 'col-1',
  title: 'Write tests',
  description: 'Original description',
  position: 0,
  assigneeId: null,
  dueDate: null,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
}

function renderWithProviders(onClose = vi.fn()) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return { queryClient, ...render(
    <QueryClientProvider client={queryClient}>
      <CardDetailModal card={card} onClose={onClose} />
    </QueryClientProvider>,
  ) }
}

describe('CardDetailModal', () => {
  beforeEach(() => {
    vi.mocked(cardsApi.updateCard).mockReset()
    vi.mocked(cardsApi.deleteCard).mockReset()
  })

  it('shows the current title and description', () => {
    renderWithProviders()

    expect(screen.getByDisplayValue('Write tests')).toBeInTheDocument()
    expect(screen.getByDisplayValue('Original description')).toBeInTheDocument()
  })

  it('saves edits via updateCard', async () => {
    vi.mocked(cardsApi.updateCard).mockResolvedValue({ ...card, title: 'Updated title' })
    const onClose = vi.fn()
    renderWithProviders(onClose)

    await userEvent.clear(screen.getByLabelText(/título/i))
    await userEvent.type(screen.getByLabelText(/título/i), 'Updated title')
    await userEvent.click(screen.getByRole('button', { name: /salvar/i }))

    await waitFor(() =>
      expect(cardsApi.updateCard).toHaveBeenCalledWith('card-1', 'Updated title', 'Original description', null, null),
    )
    expect(onClose).toHaveBeenCalled()
  })

  it('deletes the card via deleteCard', async () => {
    vi.mocked(cardsApi.deleteCard).mockResolvedValue(undefined)
    const onClose = vi.fn()
    renderWithProviders(onClose)

    await userEvent.click(screen.getByRole('button', { name: /excluir/i }))

    await waitFor(() => expect(cardsApi.deleteCard).toHaveBeenCalledWith('card-1'))
    expect(onClose).toHaveBeenCalled()
  })
})
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/CardDetailModal.test.tsx`
Expected: FAIL — `./CardDetailModal` module not found.

- [ ] **Step 7: Implement `CardDetailModal`**

`frontend/src/features/board/CardDetailModal.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateCard, deleteCard, type Card } from '../../api/cards'
import { boardKeys } from '../../lib/queryKeys'
import Modal from '../../components/ui/Modal'

interface CardDetailModalProps {
  card: Card
  boardId: string
  onClose: () => void
}

export default function CardDetailModal({ card, boardId, onClose }: CardDetailModalProps) {
  const queryClient = useQueryClient()
  const [title, setTitle] = useState(card.title)
  const [description, setDescription] = useState(card.description)

  const updateMutation = useMutation({
    mutationFn: () => updateCard(card.id, title, description, card.assigneeId, card.dueDate),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
      onClose()
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteCard(card.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
      onClose()
    },
  })

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    updateMutation.mutate()
  }

  return (
    <Modal title="Detalhes do card" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        {(updateMutation.isError || deleteMutation.isError) && (
          <p className="rounded bg-red-50 p-2 text-sm text-red-700">Não foi possível salvar as alterações.</p>
        )}
        <div>
          <label htmlFor="card-title" className="block text-sm font-medium text-gray-700">
            Título
          </label>
          <input
            id="card-title"
            type="text"
            required
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div>
          <label htmlFor="card-description" className="block text-sm font-medium text-gray-700">
            Descrição
          </label>
          <textarea
            id="card-description"
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={4}
            className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
          />
        </div>
        <div className="flex items-center justify-between">
          <button
            type="button"
            onClick={() => deleteMutation.mutate()}
            disabled={deleteMutation.isPending}
            className="rounded border border-red-300 px-3 py-1.5 text-sm text-red-700 hover:bg-red-50 disabled:opacity-50"
          >
            Excluir
          </button>
          <button
            type="submit"
            disabled={updateMutation.isPending}
            className="rounded bg-blue-600 px-4 py-1.5 text-sm text-white disabled:opacity-50"
          >
            Salvar
          </button>
        </div>
      </form>
    </Modal>
  )
}
```

Update the test file from Step 5 to pass `boardId="board-1"` as a prop to `<CardDetailModal card={card} boardId="board-1" onClose={onClose} />` in both places it's rendered, before running it.

- [ ] **Step 8: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/CardDetailModal.test.tsx`
Expected: PASS

- [ ] **Step 9: Write the failing test for `Column`**

`frontend/src/features/board/Column.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Column from './Column'
import * as columnsApi from '../../api/columns'
import * as cardsApi from '../../api/cards'
import type { ColumnWithCards } from '../../api/columns'

vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, renameColumn: vi.fn(), deleteColumn: vi.fn() }
})
vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, createCard: vi.fn() }
})

const column: ColumnWithCards = {
  id: 'col-1',
  boardId: 'board-1',
  title: 'To do',
  position: 0,
  createdAt: '2026-08-12T00:00:00Z',
  updatedAt: '2026-08-12T00:00:00Z',
  cards: [
    {
      id: 'card-1',
      columnId: 'col-1',
      title: 'Write tests',
      description: '',
      position: 0,
      assigneeId: null,
      dueDate: null,
      createdAt: '2026-08-12T00:00:00Z',
      updatedAt: '2026-08-12T00:00:00Z',
    },
  ],
}

function renderWithProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <Column column={column} boardId="board-1" />
    </QueryClientProvider>,
  )
}

describe('Column', () => {
  beforeEach(() => {
    vi.mocked(columnsApi.renameColumn).mockReset()
    vi.mocked(columnsApi.deleteColumn).mockReset()
    vi.mocked(cardsApi.createCard).mockReset()
  })

  it('renders the column title and its cards', () => {
    renderWithProviders()

    expect(screen.getByText('To do')).toBeInTheDocument()
    expect(screen.getByText('Write tests')).toBeInTheDocument()
  })

  it('creates a card via the add-card form', async () => {
    vi.mocked(cardsApi.createCard).mockResolvedValue({ ...column.cards[0], id: 'card-2', title: 'New card' })
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /adicionar card/i }))
    await userEvent.type(screen.getByLabelText(/título do card/i), 'New card')
    await userEvent.click(screen.getByRole('button', { name: /^adicionar$/i }))

    await waitFor(() => expect(cardsApi.createCard).toHaveBeenCalledWith('col-1', 'New card'))
  })

  it('renames the column via the menu', async () => {
    vi.mocked(columnsApi.renameColumn).mockResolvedValue({ ...column, title: 'Doing' })
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /opções da coluna/i }))
    await userEvent.click(screen.getByRole('button', { name: /renomear/i }))
    const input = screen.getByDisplayValue('To do')
    await userEvent.clear(input)
    await userEvent.type(input, 'Doing{Enter}')

    await waitFor(() => expect(columnsApi.renameColumn).toHaveBeenCalledWith('board-1', 'col-1', 'Doing'))
  })

  it('deletes the column via the menu', async () => {
    vi.mocked(columnsApi.deleteColumn).mockResolvedValue(undefined)
    renderWithProviders()

    await userEvent.click(screen.getByRole('button', { name: /opções da coluna/i }))
    await userEvent.click(screen.getByRole('button', { name: /excluir coluna/i }))

    await waitFor(() => expect(columnsApi.deleteColumn).toHaveBeenCalledWith('board-1', 'col-1'))
  })
})
```

- [ ] **Step 10: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/Column.test.tsx`
Expected: FAIL — `./Column` module not found.

- [ ] **Step 11: Implement `Column`**

`frontend/src/features/board/Column.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { renameColumn, deleteColumn, type ColumnWithCards } from '../../api/columns'
import { createCard } from '../../api/cards'
import { boardKeys } from '../../lib/queryKeys'
import CardItem from './CardItem'
import CardDetailModal from './CardDetailModal'

interface ColumnProps {
  column: ColumnWithCards
  boardId: string
}

export default function Column({ column, boardId }: ColumnProps) {
  const queryClient = useQueryClient()
  const [menuOpen, setMenuOpen] = useState(false)
  const [isRenaming, setIsRenaming] = useState(false)
  const [title, setTitle] = useState(column.title)
  const [isAddingCard, setIsAddingCard] = useState(false)
  const [newCardTitle, setNewCardTitle] = useState('')
  const [selectedCardId, setSelectedCardId] = useState<string | null>(null)

  function invalidate() {
    queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
  }

  const renameMutation = useMutation({
    mutationFn: (newTitle: string) => renameColumn(boardId, column.id, newTitle),
    onSuccess: () => {
      invalidate()
      setIsRenaming(false)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteColumn(boardId, column.id),
    onSuccess: invalidate,
  })

  const createCardMutation = useMutation({
    mutationFn: (cardTitle: string) => createCard(column.id, cardTitle),
    onSuccess: () => {
      invalidate()
      setIsAddingCard(false)
      setNewCardTitle('')
    },
  })

  function handleRenameSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    renameMutation.mutate(title)
  }

  function handleAddCardSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createCardMutation.mutate(newCardTitle)
  }

  const selectedCard = column.cards.find((c) => c.id === selectedCardId) ?? null

  return (
    <div className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">
      <div className="mb-2 flex items-center justify-between">
        {isRenaming ? (
          <form onSubmit={handleRenameSubmit} className="flex-1">
            <input
              type="text"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              onBlur={handleRenameSubmit}
              autoFocus
              className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
            />
          </form>
        ) : (
          <h3 className="font-medium text-gray-900">{column.title}</h3>
        )}
        <div className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen((open) => !open)}
            aria-label="Opções da coluna"
            className="px-1 text-gray-500 hover:text-gray-800"
          >
            ⋯
          </button>
          {menuOpen && (
            <div className="absolute right-0 z-10 mt-1 w-36 rounded border border-gray-200 bg-white shadow">
              <button
                type="button"
                onClick={() => {
                  setIsRenaming(true)
                  setMenuOpen(false)
                }}
                className="block w-full px-3 py-2 text-left text-sm hover:bg-gray-50"
              >
                Renomear
              </button>
              <button
                type="button"
                onClick={() => {
                  deleteMutation.mutate()
                  setMenuOpen(false)
                }}
                className="block w-full px-3 py-2 text-left text-sm text-red-700 hover:bg-red-50"
              >
                Excluir coluna
              </button>
            </div>
          )}
        </div>
      </div>

      <div className="flex flex-col gap-2">
        {column.cards.map((card) => (
          <CardItem key={card.id} card={card} onClick={() => setSelectedCardId(card.id)} />
        ))}
      </div>

      {isAddingCard ? (
        <form onSubmit={handleAddCardSubmit} className="mt-2 space-y-2">
          <label htmlFor={`new-card-${column.id}`} className="sr-only">
            Título do card
          </label>
          <input
            id={`new-card-${column.id}`}
            type="text"
            required
            value={newCardTitle}
            onChange={(event) => setNewCardTitle(event.target.value)}
            className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
            autoFocus
          />
          <div className="flex gap-2">
            <button
              type="submit"
              disabled={createCardMutation.isPending}
              className="rounded bg-blue-600 px-3 py-1 text-sm text-white disabled:opacity-50"
            >
              Adicionar
            </button>
            <button
              type="button"
              onClick={() => setIsAddingCard(false)}
              className="rounded px-3 py-1 text-sm text-gray-600 hover:bg-gray-200"
            >
              Cancelar
            </button>
          </div>
        </form>
      ) : (
        <button
          type="button"
          onClick={() => setIsAddingCard(true)}
          className="mt-2 rounded px-2 py-1 text-left text-sm text-gray-600 hover:bg-gray-200"
        >
          + Adicionar card
        </button>
      )}

      {selectedCard && (
        <CardDetailModal card={selectedCard} boardId={boardId} onClose={() => setSelectedCardId(null)} />
      )}
    </div>
  )
}
```

- [ ] **Step 12: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/Column.test.tsx`
Expected: PASS

- [ ] **Step 13: Write the failing test for `BoardPage`**

`frontend/src/features/board/BoardPage.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import BoardPage from './BoardPage'
import * as columnsApi from '../../api/columns'

vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, listColumns: vi.fn(), createColumn: vi.fn() }
})

function renderWithProviders(boardId = 'board-1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/boards/${boardId}`]}>
        <Routes>
          <Route path="/boards/:boardId" element={<BoardPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('BoardPage', () => {
  beforeEach(() => {
    vi.mocked(columnsApi.listColumns).mockReset()
    vi.mocked(columnsApi.createColumn).mockReset()
  })

  it('renders the fetched columns', async () => {
    vi.mocked(columnsApi.listColumns).mockResolvedValue([
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ])

    renderWithProviders()

    expect(await screen.findByText('To do')).toBeInTheDocument()
  })

  it('shows a retry option when the fetch fails', async () => {
    vi.mocked(columnsApi.listColumns).mockRejectedValue(new Error('network down'))

    renderWithProviders()

    expect(await screen.findByRole('button', { name: /tentar novamente/i })).toBeInTheDocument()
  })

  it('creates a column via the add-column form', async () => {
    vi.mocked(columnsApi.listColumns).mockResolvedValue([])
    vi.mocked(columnsApi.createColumn).mockResolvedValue({
      id: 'col-1',
      boardId: 'board-1',
      title: 'Backlog',
      position: 0,
      createdAt: '',
      updatedAt: '',
    })

    renderWithProviders()
    await waitFor(() => expect(columnsApi.listColumns).toHaveBeenCalled())

    await userEvent.click(screen.getByRole('button', { name: /adicionar coluna/i }))
    await userEvent.type(screen.getByLabelText(/título da coluna/i), 'Backlog')
    await userEvent.click(screen.getByRole('button', { name: /^adicionar$/i }))

    await waitFor(() => expect(columnsApi.createColumn).toHaveBeenCalledWith('board-1', 'Backlog'))
  })
})
```

- [ ] **Step 14: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/BoardPage.test.tsx`
Expected: FAIL — `./BoardPage` module not found.

- [ ] **Step 15: Implement `BoardPage`**

`frontend/src/features/board/BoardPage.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listColumns, createColumn } from '../../api/columns'
import Column from './Column'

export default function BoardPage() {
  const { boardId } = useParams<{ boardId: string }>()
  const queryClient = useQueryClient()
  const [isAddingColumn, setIsAddingColumn] = useState(false)
  const [newColumnTitle, setNewColumnTitle] = useState('')

  if (!boardId) {
    return null
  }

  const { data: columns, isPending, isError, refetch } = useQuery({
    queryKey: boardKeys.columns(boardId),
    queryFn: () => listColumns(boardId),
  })

  const createColumnMutation = useMutation({
    mutationFn: (title: string) => createColumn(boardId, title),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) })
      setIsAddingColumn(false)
      setNewColumnTitle('')
    },
  })

  function handleAddColumnSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    createColumnMutation.mutate(newColumnTitle)
  }

  if (isPending) {
    return (
      <div className="flex gap-4">
        {[0, 1, 2].map((i) => (
          <div key={i} className="h-64 w-72 shrink-0 animate-pulse rounded-lg bg-gray-200" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-start gap-3">
        <p className="text-sm text-red-700">Não foi possível carregar as colunas.</p>
        <button
          type="button"
          onClick={() => refetch()}
          className="rounded border border-gray-300 px-3 py-1 text-sm text-gray-700 hover:bg-gray-100"
        >
          Tentar novamente
        </button>
      </div>
    )
  }

  return (
    <div className="flex gap-4 overflow-x-auto pb-4">
      {columns.map((column) => (
        <Column key={column.id} column={column} boardId={boardId} />
      ))}

      <div className="w-72 shrink-0">
        {isAddingColumn ? (
          <form onSubmit={handleAddColumnSubmit} className="space-y-2 rounded-lg bg-gray-100 p-3">
            <label htmlFor="new-column-title" className="sr-only">
              Título da coluna
            </label>
            <input
              id="new-column-title"
              type="text"
              required
              value={newColumnTitle}
              onChange={(event) => setNewColumnTitle(event.target.value)}
              className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
              autoFocus
            />
            <div className="flex gap-2">
              <button
                type="submit"
                disabled={createColumnMutation.isPending}
                className="rounded bg-blue-600 px-3 py-1 text-sm text-white disabled:opacity-50"
              >
                Adicionar
              </button>
              <button
                type="button"
                onClick={() => setIsAddingColumn(false)}
                className="rounded px-3 py-1 text-sm text-gray-600 hover:bg-gray-200"
              >
                Cancelar
              </button>
            </div>
          </form>
        ) : (
          <button
            type="button"
            onClick={() => setIsAddingColumn(true)}
            className="w-full rounded-lg border-2 border-dashed border-gray-300 p-3 text-sm text-gray-500 hover:border-gray-400"
          >
            + Adicionar coluna
          </button>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 16: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/BoardPage.test.tsx`
Expected: PASS

- [ ] **Step 17: Wire the route**

In `frontend/src/routes/router.tsx`, add the import:

```tsx
import BoardPage from '../features/board/BoardPage'
```

and add `/boards/:boardId` as a sibling of `/` inside the `AppLayout` children array:

```tsx
        children: [
          { path: '/', element: <BoardListPage /> },
          { path: '/boards/:boardId', element: <BoardPage /> },
        ],
```

- [ ] **Step 18: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green, 0 new lint warnings, then:

```bash
git add src/features/board/BoardPage.tsx src/features/board/BoardPage.test.tsx src/features/board/Column.tsx src/features/board/Column.test.tsx src/features/board/CardItem.tsx src/features/board/CardItem.test.tsx src/features/board/CardDetailModal.tsx src/features/board/CardDetailModal.test.tsx src/routes/router.tsx
git commit -m "feat(frontend): add kanban board view with column and card CRUD"
```

---

## Task 6: Frontend — realtime sync via WebSocket

**Files:**
- Create: `frontend/src/features/board/useBoardRealtime.ts`
- Create: `frontend/src/features/board/useBoardRealtime.test.ts`
- Modify: `frontend/src/features/board/BoardPage.tsx`

**Interfaces:**
- Consumes: `boardKeys` (Task 3), `getAccessToken` from `frontend/src/api/client.ts` (Phase 5), `env.API_URL` from `frontend/src/lib/env.ts` (Phase 5).
- Produces: `useBoardRealtime(boardId: string): void` — a hook called from `BoardPage`, no return value. It owns the WebSocket lifecycle and patches `boardKeys.columns(boardId)` in the query cache directly.

- [ ] **Step 1: Write the failing test for `useBoardRealtime`**

`frontend/src/features/board/useBoardRealtime.test.ts`:

```ts
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useBoardRealtime } from './useBoardRealtime'
import { boardKeys } from '../../lib/queryKeys'
import * as client from '../../api/client'
import type { ColumnWithCards } from '../../api/columns'

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, getAccessToken: vi.fn(() => 'test-token') }
})

class FakeWebSocket {
  static instances: FakeWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  url: string

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  close() {
    this.closed = true
    this.onclose?.()
  }

  emit(type: string, data: unknown) {
    this.onmessage?.({ data: JSON.stringify({ type, board_id: 'board-1', data }) })
  }
}

describe('useBoardRealtime', () => {
  let queryClient: QueryClient
  let wrapper: ({ children }: { children: ReactNode }) => JSX.Element

  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.stubGlobal('WebSocket', FakeWebSocket)
    queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    wrapper = ({ children }) => <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens a WebSocket to the board endpoint with the access token', () => {
    renderHook(() => useBoardRealtime('board-1'), { wrapper })

    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].url).toContain('/boards/board-1/ws?token=test-token')
  })

  it('appends a card on card.created', () => {
    const initial: ColumnWithCards[] = [
      { id: 'col-1', boardId: 'board-1', title: 'To do', position: 0, createdAt: '', updatedAt: '', cards: [] },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('card.created', {
      id: 'card-1',
      column_id: 'col-1',
      title: 'New card',
      description: '',
      position: 0,
      assignee_id: null,
      due_date: null,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(1)
    expect(data?.[0].cards[0].title).toBe('New card')
  })

  it('removes a card on card.deleted', () => {
    const initial: ColumnWithCards[] = [
      {
        id: 'col-1',
        boardId: 'board-1',
        title: 'To do',
        position: 0,
        createdAt: '',
        updatedAt: '',
        cards: [
          {
            id: 'card-1',
            columnId: 'col-1',
            title: 'Old card',
            description: '',
            position: 0,
            assigneeId: null,
            dueDate: null,
            createdAt: '',
            updatedAt: '',
          },
        ],
      },
    ]
    queryClient.setQueryData(boardKeys.columns('board-1'), initial)

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('card.deleted', { id: 'card-1', column_id: 'col-1' })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data?.[0].cards).toHaveLength(0)
  })

  it('appends a column on column.created', () => {
    queryClient.setQueryData(boardKeys.columns('board-1'), [] as ColumnWithCards[])

    renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]
    ws.emit('column.created', {
      id: 'col-2',
      board_id: 'board-1',
      title: 'Done',
      position: 1,
      created_at: '',
      updated_at: '',
    })

    const data = queryClient.getQueryData<ColumnWithCards[]>(boardKeys.columns('board-1'))
    expect(data).toHaveLength(1)
    expect(data?.[0].title).toBe('Done')
  })

  it('closes the socket on unmount', () => {
    const { unmount } = renderHook(() => useBoardRealtime('board-1'), { wrapper })
    const ws = FakeWebSocket.instances[0]

    unmount()

    expect(ws.closed).toBe(true)
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/useBoardRealtime.test.ts`
Expected: FAIL — `./useBoardRealtime` module not found.

- [ ] **Step 3: Implement `useBoardRealtime`**

`frontend/src/features/board/useBoardRealtime.ts`:

```ts
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { getAccessToken } from '../../api/client'
import { env } from '../../lib/env'
import { toCard, type CardBody, type Card } from '../../api/cards'
import type { ColumnWithCards } from '../../api/columns'

interface RealtimeEvent {
  type: string
  board_id: string
  data: unknown
}

interface ColumnEventPayload {
  id: string
  board_id: string
  title: string
  position: number
  created_at: string
  updated_at: string
}

interface ColumnDeletedPayload {
  id: string
  board_id: string
}

interface ColumnsReorderedPayload {
  board_id: string
  column_ids: string[]
}

interface CardDeletedPayload {
  id: string
  column_id: string
}

const RECONNECT_DELAYS_MS = [1000, 2000, 4000, 8000, 10000]

function wsURL(boardId: string): string {
  const httpURL = new URL(`${env.API_URL}/boards/${boardId}/ws`)
  const wsProtocol = httpURL.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = getAccessToken() ?? ''
  return `${wsProtocol}//${httpURL.host}/boards/${boardId}/ws?token=${encodeURIComponent(token)}`
}

export function useBoardRealtime(boardId: string): void {
  const queryClient = useQueryClient()

  useEffect(() => {
    let socket: WebSocket | null = null
    let reconnectAttempt = 0
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let stopped = false

    function patchColumns(updater: (columns: ColumnWithCards[]) => ColumnWithCards[]) {
      queryClient.setQueryData<ColumnWithCards[]>(boardKeys.columns(boardId), (current) =>
        updater(current ?? []),
      )
    }

    function handleEvent(event: RealtimeEvent) {
      switch (event.type) {
        case 'column.created': {
          const payload = event.data as ColumnEventPayload
          patchColumns((columns) => [
            ...columns,
            {
              id: payload.id,
              boardId: payload.board_id,
              title: payload.title,
              position: payload.position,
              createdAt: payload.created_at,
              updatedAt: payload.updated_at,
              cards: [],
            },
          ])
          break
        }
        case 'column.updated': {
          const payload = event.data as ColumnEventPayload
          patchColumns((columns) =>
            columns.map((column) =>
              column.id === payload.id ? { ...column, title: payload.title } : column,
            ),
          )
          break
        }
        case 'column.deleted': {
          const payload = event.data as ColumnDeletedPayload
          patchColumns((columns) => columns.filter((column) => column.id !== payload.id))
          break
        }
        case 'column.reordered': {
          const payload = event.data as ColumnsReorderedPayload
          patchColumns((columns) => {
            const byId = new Map(columns.map((column) => [column.id, column]))
            return payload.column_ids
              .map((id) => byId.get(id))
              .filter((column): column is ColumnWithCards => column !== undefined)
          })
          break
        }
        case 'card.created': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) =>
            columns.map((column) =>
              column.id === card.columnId ? { ...column, cards: [...column.cards, card] } : column,
            ),
          )
          break
        }
        case 'card.updated': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) =>
            columns.map((column) =>
              column.id === card.columnId
                ? { ...column, cards: column.cards.map((c) => (c.id === card.id ? card : c)) }
                : column,
            ),
          )
          break
        }
        case 'card.deleted': {
          const payload = event.data as CardDeletedPayload
          patchColumns((columns) =>
            columns.map((column) => ({
              ...column,
              cards: column.cards.filter((c) => c.id !== payload.id),
            })),
          )
          break
        }
        case 'card.moved': {
          const card = toCard(event.data as CardBody)
          patchColumns((columns) => {
            const withoutCard = columns.map((column) => ({
              ...column,
              cards: column.cards.filter((c) => c.id !== card.id),
            }))
            return withoutCard.map((column) =>
              column.id === card.columnId
                ? { ...column, cards: insertAt(column.cards, card, card.position) }
                : column,
            )
          })
          break
        }
        default:
          break
      }
    }

    function connect() {
      if (stopped) return
      socket = new WebSocket(wsURL(boardId))
      socket.onmessage = (message) => {
        try {
          handleEvent(JSON.parse(message.data as string) as RealtimeEvent)
        } catch {
          // Ignore malformed frames rather than crashing the socket handler.
        }
      }
      socket.onopen = () => {
        reconnectAttempt = 0
      }
      socket.onclose = () => {
        if (stopped) return
        const delay = RECONNECT_DELAYS_MS[Math.min(reconnectAttempt, RECONNECT_DELAYS_MS.length - 1)]
        reconnectAttempt += 1
        reconnectTimer = setTimeout(connect, delay)
      }
    }

    connect()

    return () => {
      stopped = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [boardId, queryClient])
}

function insertAt(cards: Card[], card: Card, index: number): Card[] {
  const clamped = Math.max(0, Math.min(index, cards.length))
  const next = [...cards]
  next.splice(clamped, 0, card)
  return next
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/useBoardRealtime.test.ts`
Expected: PASS

- [ ] **Step 5: Wire the hook into `BoardPage`**

In `frontend/src/features/board/BoardPage.tsx`, add the import:

```tsx
import { useBoardRealtime } from './useBoardRealtime'
```

and call it right after the early `if (!boardId) return null` guard (it must run unconditionally once `boardId` is known, so add it as the first hook-like statement after that guard, before the `useQuery` call — since `useBoardRealtime` itself doesn't call any hooks conditionally, this ordering is safe):

```tsx
  if (!boardId) {
    return null
  }

  useBoardRealtime(boardId)

  const { data: columns, isPending, isError, refetch } = useQuery({
```

- [ ] **Step 6: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green, 0 new lint warnings, then:

```bash
git add src/features/board/useBoardRealtime.ts src/features/board/useBoardRealtime.test.ts src/features/board/BoardPage.tsx
git commit -m "feat(frontend): sync board view with realtime WebSocket events"
```

---

## Task 7: Frontend — drag-and-drop for cards and columns

**Files:**
- Modify: `frontend/src/features/board/BoardPage.tsx`
- Modify: `frontend/src/features/board/BoardPage.test.tsx`
- Modify: `frontend/src/features/board/Column.tsx`
- Modify: `frontend/src/features/board/Column.test.tsx`
- Modify: `frontend/src/features/board/CardItem.tsx`
- Modify: `frontend/src/features/board/CardItem.test.tsx`

**Interfaces:**
- Consumes: `moveCard`, `reorderColumns` (Task 3), `@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities` (installed in Task 3).
- Produces: no new exports — this task adds drag behavior to the existing `BoardPage`/`Column`/`CardItem` components. Deliberately simplified: no live cross-column preview while dragging (the dragged item follows the cursor via `DragOverlay`, but the underlying lists only reorder on drop) — this avoids the most error-prone part of multi-container dnd-kit setups while still being fully functional.

- [ ] **Step 1: Add sortable behavior to `CardItem`**

In `frontend/src/features/board/CardItem.tsx`, add drag-handle support via `useSortable`. Replace the whole file:

```tsx
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { Card } from '../../api/cards'

interface CardItemProps {
  card: Card
  onClick: () => void
}

export default function CardItem({ card, onClick }: CardItemProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: card.id,
    data: { type: 'card', columnId: card.columnId },
  })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  }

  return (
    <button
      ref={setNodeRef}
      style={style}
      type="button"
      onClick={onClick}
      {...attributes}
      {...listeners}
      className="w-full rounded border border-gray-200 bg-white p-3 text-left shadow-sm hover:border-blue-400"
    >
      <p className="font-medium text-gray-900">{card.title}</p>
      {card.description && (
        <p className="mt-1 line-clamp-2 text-sm text-gray-600">{card.description}</p>
      )}
      {card.dueDate && (
        <span className="mt-2 inline-block rounded bg-gray-100 px-2 py-0.5 text-xs text-gray-600">
          {card.dueDate.slice(0, 10)}
        </span>
      )}
    </button>
  )
}
```

In `frontend/src/features/board/CardItem.test.tsx`, wrap every `render(<CardItem ... />)` call in a `DndContext` + `SortableContext` (a bare `useSortable` call throws outside one). Add these imports:

```tsx
import { DndContext } from '@dnd-kit/core'
import { SortableContext } from '@dnd-kit/sortable'
```

and a helper used by all three existing tests:

```tsx
function renderCard(card: Card, onClick: () => void) {
  return render(
    <DndContext>
      <SortableContext items={[card.id]}>
        <CardItem card={card} onClick={onClick} />
      </SortableContext>
    </DndContext>,
  )
}
```

then replace each `render(<CardItem card={...} onClick={...} />)` call with `renderCard(...)` accordingly, keeping every existing assertion unchanged.

- [ ] **Step 2: Run the test to verify it still passes**

Run: `npm test -- --run src/features/board/CardItem.test.tsx`
Expected: PASS

- [ ] **Step 3: Add sortable-column support to `Column`**

In `frontend/src/features/board/Column.tsx`, wrap the card list in a `SortableContext` and make the column header itself draggable (as a column-reorder handle). Add these imports at the top:

```tsx
import { SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { useDroppable } from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
```

Inside the `Column` function, right after the existing `const selectedCard = ...` line, add:

```tsx
  const { attributes, listeners, setNodeRef: setColumnNodeRef, transform, transition } = useSortable({
    id: column.id,
    data: { type: 'column' },
  })
  const columnStyle = { transform: CSS.Transform.toString(transform), transition }

  const { setNodeRef: setDroppableRef } = useDroppable({
    id: `column-droppable-${column.id}`,
    data: { type: 'column', columnId: column.id },
  })
```

Change the outermost returned `<div className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">` to:

```tsx
    <div ref={setColumnNodeRef} style={columnStyle} className="flex w-72 shrink-0 flex-col rounded-lg bg-gray-100 p-3">
```

Make the header drag handle (the `<h3>`/rename form row) a drag source by spreading `{...attributes} {...listeners}` onto the wrapping `<div className="mb-2 flex items-center justify-between">`:

```tsx
      <div className="mb-2 flex items-center justify-between" {...attributes} {...listeners}>
```

Wrap the card-list `<div className="flex flex-col gap-2">...</div>` block in a droppable + sortable context:

```tsx
      <div ref={setDroppableRef} className="flex flex-col gap-2">
        <SortableContext items={column.cards.map((c) => c.id)} strategy={verticalListSortingStrategy}>
          {column.cards.map((card) => (
            <CardItem key={card.id} card={card} onClick={() => setSelectedCardId(card.id)} />
          ))}
        </SortableContext>
      </div>
```

The `⋯` options button's click handler still works normally since drag listeners only activate past `PointerSensor`'s activation distance (configured in `BoardPage`, Step 5 below), not on a plain click.

In `frontend/src/features/board/Column.test.tsx`, wrap the `renderWithProviders` helper's tree in `DndContext` + a top-level `SortableContext` containing `[column.id]`, matching the pattern from Step 1:

```tsx
import { DndContext } from '@dnd-kit/core'
import { SortableContext } from '@dnd-kit/sortable'
```

```tsx
function renderWithProviders() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <DndContext>
        <SortableContext items={[column.id]}>
          <Column column={column} boardId="board-1" />
        </SortableContext>
      </DndContext>
    </QueryClientProvider>,
  )
}
```

- [ ] **Step 4: Run the test to verify it still passes**

Run: `npm test -- --run src/features/board/Column.test.tsx`
Expected: PASS

- [ ] **Step 5: Wrap `BoardPage`'s columns in `DndContext` and handle drag end**

In `frontend/src/features/board/BoardPage.tsx`, add these imports:

```tsx
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  closestCorners,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { SortableContext, horizontalListSortingStrategy, arrayMove } from '@dnd-kit/sortable'
import { moveCard } from '../../api/cards'
import { reorderColumns } from '../../api/columns'
import CardItem from './CardItem'
import type { ColumnWithCards } from '../../api/columns'
```

Add state and mutations inside the component, after the existing `createColumnMutation` block:

```tsx
  const [activeCard, setActiveCard] = useState<ColumnWithCards['cards'][number] | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 5 } }))

  const moveCardMutation = useMutation({
    mutationFn: ({ cardId, targetColumnId, position }: { cardId: string; targetColumnId: string; position: number }) =>
      moveCard(cardId, targetColumnId, position),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) }),
  })

  const reorderColumnsMutation = useMutation({
    mutationFn: (columnIds: string[]) => reorderColumns(boardId, columnIds),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.columns(boardId) }),
  })

  function handleDragStart(event: DragStartEvent) {
    if (event.active.data.current?.type !== 'card') return
    const card = columns?.flatMap((c) => c.cards).find((c) => c.id === event.active.id)
    setActiveCard(card ?? null)
  }

  function handleDragEnd(event: DragEndEvent) {
    setActiveCard(null)
    const { active, over } = event
    if (!over || !columns) return

    if (active.data.current?.type === 'column') {
      const oldIndex = columns.findIndex((c) => c.id === active.id)
      const newIndex = columns.findIndex((c) => c.id === over.id)
      if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) return
      const reordered = arrayMove(columns, oldIndex, newIndex)
      reorderColumnsMutation.mutate(reordered.map((c) => c.id))
      return
    }

    if (active.data.current?.type === 'card') {
      const cardId = String(active.id)
      const overData = over.data.current as { type: string; columnId?: string } | undefined
      const targetColumnId =
        overData?.type === 'column'
          ? overData.columnId!
          : (columns.find((c) => c.cards.some((card) => card.id === over.id))?.id ?? null)
      if (!targetColumnId) return

      const targetColumn = columns.find((c) => c.id === targetColumnId)
      if (!targetColumn) return

      const overIndex = targetColumn.cards.findIndex((card) => card.id === over.id)
      const position = overIndex === -1 ? targetColumn.cards.length : overIndex

      moveCardMutation.mutate({ cardId, targetColumnId, position })
    }
  }
```

Replace the main returned `<div className="flex gap-4 overflow-x-auto pb-4">...</div>` block with a `DndContext`-wrapped version:

```tsx
  return (
    <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <div className="flex gap-4 overflow-x-auto pb-4">
        <SortableContext items={columns.map((c) => c.id)} strategy={horizontalListSortingStrategy}>
          {columns.map((column) => (
            <Column key={column.id} column={column} boardId={boardId} />
          ))}
        </SortableContext>

        <div className="w-72 shrink-0">
          {isAddingColumn ? (
            <form onSubmit={handleAddColumnSubmit} className="space-y-2 rounded-lg bg-gray-100 p-3">
              <label htmlFor="new-column-title" className="sr-only">
                Título da coluna
              </label>
              <input
                id="new-column-title"
                type="text"
                required
                value={newColumnTitle}
                onChange={(event) => setNewColumnTitle(event.target.value)}
                className="w-full rounded border border-gray-300 px-2 py-1 text-sm"
                autoFocus
              />
              <div className="flex gap-2">
                <button
                  type="submit"
                  disabled={createColumnMutation.isPending}
                  className="rounded bg-blue-600 px-3 py-1 text-sm text-white disabled:opacity-50"
                >
                  Adicionar
                </button>
                <button
                  type="button"
                  onClick={() => setIsAddingColumn(false)}
                  className="rounded px-3 py-1 text-sm text-gray-600 hover:bg-gray-200"
                >
                  Cancelar
                </button>
              </div>
            </form>
          ) : (
            <button
              type="button"
              onClick={() => setIsAddingColumn(true)}
              className="w-full rounded-lg border-2 border-dashed border-gray-300 p-3 text-sm text-gray-500 hover:border-gray-400"
            >
              + Adicionar coluna
            </button>
          )}
        </div>
      </div>

      <DragOverlay>{activeCard && <CardItem card={activeCard} onClick={() => {}} />}</DragOverlay>
    </DndContext>
  )
```

Note the `DragOverlay`'s `CardItem` renders outside any `SortableContext` — that's fine, `useSortable` inside it will simply behave as a non-sortable draggable clone since `DragOverlay` doesn't participate in the sortable context lookup.

- [ ] **Step 6: Update `BoardPage.test.tsx`'s mocks for the new imports**

`BoardPage.tsx` now imports `moveCard` from `../../api/cards` and `reorderColumns` from `../../api/columns`. Update the existing `vi.mock` factories in `frontend/src/features/board/BoardPage.test.tsx` so both are mocked (matching the pattern already used for `listColumns`/`createColumn`):

```tsx
vi.mock('../../api/columns', async () => {
  const actual = await vi.importActual<typeof import('../../api/columns')>('../../api/columns')
  return { ...actual, listColumns: vi.fn(), createColumn: vi.fn(), reorderColumns: vi.fn() }
})
vi.mock('../../api/cards', async () => {
  const actual = await vi.importActual<typeof import('../../api/cards')>('../../api/cards')
  return { ...actual, moveCard: vi.fn() }
})
```

No new automated test is added for the drag gesture itself: dnd-kit's `PointerSensor` cannot be driven through jsdom's synthetic pointer events in a way Testing Library reliably simulates, so a fabricated drag-end test would assert nothing meaningful and would violate this project's test-hygiene bar (see Global Constraints). The existing `BoardPage.test.tsx` render/create-column tests (Task 5, Step 13) already prove the `DndContext`/`SortableContext` wrappers don't break normal rendering or interaction — that, plus the manual verification in Step 7 below, is this task's coverage for the drag behavior itself.

- [ ] **Step 7: Manual verification**

```bash
cd frontend
npm run dev
```

In the browser (with the backend running and a board that has at least two columns and a couple of cards — see `backend/README.md` for `docker-compose up` instructions): drag a card within a column, drag a card to a different column, and drag a column header to reorder columns. Confirm each action persists after a page reload (i.e., the mutation actually landed, not just the optimistic-looking drag animation).

- [ ] **Step 8: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green, 0 new lint warnings, then:

```bash
git add src/features/board/BoardPage.tsx src/features/board/BoardPage.test.tsx src/features/board/Column.tsx src/features/board/Column.test.tsx src/features/board/CardItem.tsx src/features/board/CardItem.test.tsx
git commit -m "feat(frontend): add drag-and-drop for cards and columns"
```

---

## Task 8: Frontend — members panel

**Files:**
- Create: `frontend/src/features/board/MembersPanel.tsx`
- Create: `frontend/src/features/board/MembersPanel.test.tsx`
- Modify: `frontend/src/features/board/BoardPage.tsx`
- Modify: `frontend/src/features/board/BoardPage.test.tsx`

**Interfaces:**
- Consumes: `boardKeys`, `listMembers`, `inviteMember`, `removeMember`, `Member` (Task 3), `Modal` (Task 4).
- Produces: `MembersPanel` (default export, props `{ boardId: string; currentUserId: string; onClose: () => void }`) — mounted from `BoardPage` behind a "Membros" button.

- [ ] **Step 1: Write the failing test for `MembersPanel`**

`frontend/src/features/board/MembersPanel.test.tsx`:

```tsx
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import MembersPanel from './MembersPanel'
import * as boardsApi from '../../api/boards'

vi.mock('../../api/boards', async () => {
  const actual = await vi.importActual<typeof import('../../api/boards')>('../../api/boards')
  return { ...actual, listMembers: vi.fn(), inviteMember: vi.fn(), removeMember: vi.fn() }
})

const owner = { userId: 'user-1', role: 'owner' as const, name: 'Owner Person', email: 'owner@example.com', createdAt: '' }
const member = { userId: 'user-2', role: 'member' as const, name: 'Ada Lovelace', email: 'ada@example.com', createdAt: '' }

function renderWithProviders(currentUserId = 'user-1') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <MembersPanel boardId="board-1" currentUserId={currentUserId} onClose={vi.fn()} />
    </QueryClientProvider>,
  )
}

describe('MembersPanel', () => {
  beforeEach(() => {
    vi.mocked(boardsApi.listMembers).mockReset()
    vi.mocked(boardsApi.inviteMember).mockReset()
    vi.mocked(boardsApi.removeMember).mockReset()
  })

  it('renders each member with name, email, and role', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders()

    expect(await screen.findByText('Owner Person')).toBeInTheDocument()
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
  })

  it('lets the owner invite a member by email', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner]);
    vi.mocked(boardsApi.inviteMember).mockResolvedValue(member)
    renderWithProviders('user-1')

    await screen.findByText('Owner Person')
    await userEvent.type(screen.getByLabelText(/e-mail/i), 'ada@example.com')
    await userEvent.click(screen.getByRole('button', { name: /convidar/i }))

    await waitFor(() => expect(boardsApi.inviteMember).toHaveBeenCalledWith('board-1', 'ada@example.com'))
  })

  it('does not show the invite form to a non-owner', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders('user-2')

    await screen.findByText('Owner Person')
    expect(screen.queryByLabelText(/e-mail/i)).not.toBeInTheDocument()
  })

  it('lets the owner remove a non-owner member', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    vi.mocked(boardsApi.removeMember).mockResolvedValue(undefined)
    renderWithProviders('user-1')

    await screen.findByText('Ada Lovelace')
    await userEvent.click(screen.getByRole('button', { name: /remover ada lovelace/i }))

    await waitFor(() => expect(boardsApi.removeMember).toHaveBeenCalledWith('board-1', 'user-2'))
  })

  it('does not show a remove control for the owner row', async () => {
    vi.mocked(boardsApi.listMembers).mockResolvedValue([owner, member])
    renderWithProviders('user-1')

    await screen.findByText('Owner Person')
    expect(screen.queryByRole('button', { name: /remover owner person/i })).not.toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `npm test -- --run src/features/board/MembersPanel.test.tsx`
Expected: FAIL — `./MembersPanel` module not found.

- [ ] **Step 3: Implement `MembersPanel`**

`frontend/src/features/board/MembersPanel.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { boardKeys } from '../../lib/queryKeys'
import { listMembers, inviteMember, removeMember } from '../../api/boards'
import Modal from '../../components/ui/Modal'

interface MembersPanelProps {
  boardId: string
  currentUserId: string
  onClose: () => void
}

export default function MembersPanel({ boardId, currentUserId, onClose }: MembersPanelProps) {
  const queryClient = useQueryClient()
  const [email, setEmail] = useState('')

  const { data: members, isPending } = useQuery({
    queryKey: boardKeys.members(boardId),
    queryFn: () => listMembers(boardId),
  })

  const currentMember = members?.find((m) => m.userId === currentUserId)
  const isOwner = currentMember?.role === 'owner'

  const inviteMutation = useMutation({
    mutationFn: (inviteEmail: string) => inviteMember(boardId, inviteEmail),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: boardKeys.members(boardId) })
      setEmail('')
    },
  })

  const removeMutation = useMutation({
    mutationFn: (userId: string) => removeMember(boardId, userId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: boardKeys.members(boardId) }),
  })

  function handleInviteSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    inviteMutation.mutate(email)
  }

  return (
    <Modal title="Membros" onClose={onClose}>
      {isPending ? (
        <p className="text-sm text-gray-500">Carregando...</p>
      ) : (
        <ul className="space-y-2">
          {members?.map((m) => (
            <li key={m.userId} className="flex items-center justify-between rounded border border-gray-100 px-3 py-2">
              <div>
                <p className="text-sm font-medium text-gray-900">{m.name}</p>
                <p className="text-xs text-gray-500">
                  {m.email} · {m.role === 'owner' ? 'dono' : 'membro'}
                </p>
              </div>
              {isOwner && m.role !== 'owner' && (
                <button
                  type="button"
                  onClick={() => removeMutation.mutate(m.userId)}
                  aria-label={`Remover ${m.name}`}
                  className="text-sm text-red-700 hover:underline"
                >
                  Remover
                </button>
              )}
            </li>
          ))}
        </ul>
      )}

      {isOwner && (
        <form onSubmit={handleInviteSubmit} className="mt-4 flex items-end gap-2">
          <div className="flex-1">
            <label htmlFor="invite-email" className="block text-sm font-medium text-gray-700">
              E-mail
            </label>
            <input
              id="invite-email"
              type="email"
              required
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              className="mt-1 w-full rounded border border-gray-300 px-3 py-2"
            />
          </div>
          <button
            type="submit"
            disabled={inviteMutation.isPending}
            className="rounded bg-blue-600 px-3 py-2 text-sm text-white disabled:opacity-50"
          >
            Convidar
          </button>
        </form>
      )}
      {inviteMutation.isError && (
        <p className="mt-2 text-sm text-red-700">Não foi possível convidar esse e-mail.</p>
      )}
    </Modal>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `npm test -- --run src/features/board/MembersPanel.test.tsx`
Expected: PASS

- [ ] **Step 5: Wire the "Membros" button into `BoardPage`**

In `frontend/src/features/board/BoardPage.tsx`, add the import:

```tsx
import MembersPanel from './MembersPanel'
import { useAuthStore } from '../auth/useAuthStore'
```

Add state and the current user right after the existing `const [activeCard, setActiveCard] = useState...` line:

```tsx
  const [isMembersOpen, setIsMembersOpen] = useState(false)
  const currentUserId = useAuthStore((state) => state.user?.id)
```

Add a header above the existing `<DndContext ...>` return block (the component currently returns the `DndContext` directly — wrap it in a fragment with a header row above it):

```tsx
  return (
    <>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold text-gray-900">Board</h1>
        <button
          type="button"
          onClick={() => setIsMembersOpen(true)}
          className="rounded border border-gray-300 px-3 py-1.5 text-sm text-gray-700 hover:bg-gray-100"
        >
          Membros
        </button>
      </div>

      <DndContext sensors={sensors} collisionDetection={closestCorners} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        {/* ... existing DndContext content unchanged ... */}
      </DndContext>

      {isMembersOpen && currentUserId && (
        <MembersPanel boardId={boardId} currentUserId={currentUserId} onClose={() => setIsMembersOpen(false)} />
      )}
    </>
  )
```

(Keep every line inside the existing `<DndContext>...</DndContext>` block exactly as Task 7 left it — only the surrounding wrapper changes: `<div className="flex gap-4 ...">` return becomes this fragment-wrapped version, and the early-return branches for `isPending`/`isError` stay above this, unchanged.)

- [ ] **Step 6: Update `BoardPage.test.tsx` for the new header**

The existing `BoardPage.test.tsx` tests query for column titles and buttons that still exist unchanged, so they keep passing. Add one new test:

```tsx
import { useAuthStore } from '../auth/useAuthStore'

it('opens the members panel from the header button', async () => {
  useAuthStore.setState({ user: { id: 'user-1', name: 'Owner', email: 'owner@example.com' }, status: 'authenticated' })
  vi.mocked(columnsApi.listColumns).mockResolvedValue([])

  renderWithProviders()
  await waitFor(() => expect(columnsApi.listColumns).toHaveBeenCalled())

  await userEvent.click(screen.getByRole('button', { name: /membros/i }))

  expect(await screen.findByText('Membros')).toBeInTheDocument()
})
```

This test needs `listMembers` mocked too (it will be called once the panel opens) — add `vi.mock('../../api/boards', ...)` returning `{ ...actual, listMembers: vi.fn().mockResolvedValue([]) }` at the top of the file alongside the other `vi.mock` calls, and reset it in `beforeEach` like the others.

- [ ] **Step 7: Run the full frontend suite, build, and lint, then commit**

```bash
cd frontend
npm test -- --run
npm run build
npm run lint
```

All green, 0 new lint warnings, then:

```bash
git add src/features/board/MembersPanel.tsx src/features/board/MembersPanel.test.tsx src/features/board/BoardPage.tsx src/features/board/BoardPage.test.tsx
git commit -m "feat(frontend): add board members panel with invite and remove"
```

---

## Final check (not a task — run after Task 8)

```bash
cd backend && go build ./... && go vet ./... && go test ./...
cd ../frontend && npm test -- --run && npm run build && npm run lint
```

All green end to end. This is the state the final whole-branch review evaluates.
