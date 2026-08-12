# Kanvas Phase 6: Boards UI (kanban, drag-and-drop, members, realtime)

## Goal

Build the frontend experience for boards: list boards, create/rename/delete
boards, a kanban view with columns and cards (create/rename/delete/reorder),
drag-and-drop for cards and columns, member management (invite/remove by
email), and live updates across clients via the existing WebSocket hub.
Also harden the WebSocket handshake's Origin check now that a real frontend
origin exists.

## Context

Phases 1-4 (backend) are complete and merged: auth, boards/members,
columns/cards CRUD, and a realtime WebSocket hub that publishes
`column.created/updated/deleted/reordered` and
`card.created/updated/deleted/moved` events per board.

Phase 5 (frontend) is complete and merged: React 19 + TypeScript + Vite +
Tailwind v4 scaffold, an HTTP client (`frontend/src/api/client.ts`) with
in-memory access token and 401-refresh-retry, a Zustand auth store
(`frontend/src/features/auth/useAuthStore.ts`), login/register pages,
protected routing (`RequireAuth`), and `AppLayout`. The current home route
(`/`) renders a placeholder `HomePage` in `frontend/src/routes/router.tsx`
that this phase replaces.

## Backend endpoints available (already implemented, no backend CRUD work needed)

Boards (`internal/board/handler.go`):
- `GET /boards` — list boards for current user
- `POST /boards` — create board (`{title}`)
- `GET /boards/{boardID}` — get board
- `PATCH /boards/{boardID}` — rename board (`{title}`)
- `DELETE /boards/{boardID}` — delete board
- `GET /boards/{boardID}/members` — list members
- `POST /boards/{boardID}/members` — invite by email (`{email}`), owner only
- `DELETE /boards/{boardID}/members/{userID}` — remove member, owner only, cannot remove owner

Columns/cards (`internal/card/handler.go`, nested under `/boards/{boardID}`):
- `GET /boards/{boardID}/columns` — list columns (each column embeds its cards, see `toColumnView`)
- `POST /boards/{boardID}/columns` — create column (`{title}`)
- `PATCH /boards/{boardID}/columns/{columnID}` — rename column (`{title}`)
- `DELETE /boards/{boardID}/columns/{columnID}` — delete column
- `PATCH /boards/{boardID}/columns/reorder` — reorder columns (`{columnIDs: [...]}`, full permutation)
- `POST /boards/{boardID}/columns/{columnID}/cards` — create card (`{title, description?}`)
- `PATCH /boards/{boardID}/columns/{columnID}/cards/{cardID}` — update card (`{title?, description?, assigneeID?, dueDate?}`)
- `DELETE /boards/{boardID}/columns/{columnID}/cards/{cardID}` — delete card
- `PATCH /boards/{boardID}/columns/{columnID}/cards/{cardID}/move` — move card (`{columnID, position}` — target column + index)

Realtime (`internal/realtime/handler.go`):
- `GET /boards/{boardID}/ws?token={accessToken}` — WebSocket, one JSON `{type, payload}` event per message. Event types: `column.created`, `column.updated`, `column.deleted`, `column.reordered`, `card.created`, `card.updated`, `card.deleted`, `card.moved`.

All error bodies follow the existing envelope `{"error":{"code":"...","message":"..."}}`, mapped by the existing `ApiError` class.

*(Implementers: read the actual request/response field names from the
`handler.go`/`domain.go` source at implementation time — this list is for
orientation, not verbatim wire types.)*

## Architecture

- **Server state:** `@tanstack/react-query` manages boards, columns, and
  cards. New API modules `frontend/src/api/boards.ts`,
  `frontend/src/api/columns.ts`, `frontend/src/api/cards.ts` follow the
  existing `api/auth.ts` pattern (thin functions wrapping `apiFetch`).
  Zustand remains scoped to session/auth state only, unchanged.
- **Mutations:** create/rename/delete use standard invalidate-on-success.
  Card move and column reorder use optimistic updates (write the new order
  into the React Query cache immediately, roll back via `onError` using the
  snapshotted previous value if the mutation fails).
- **Realtime sync:** on entering a board, open
  `wss://.../boards/{id}/ws?token={accessToken}` (token read via the
  client's existing `getAccessToken()`). Each event patches the React Query
  cache directly via `queryClient.setQueryData` for that board's columns
  query — no refetch. Reconnect automatically on close with a simple capped
  backoff (e.g. 1s, 2s, 4s, cap 10s); no user-facing error for a dropped
  socket, since HTTP mutations keep working regardless.
- **Drag-and-drop:** `@dnd-kit/core` + `@dnd-kit/sortable`. Cards are
  sortable within and across column droppable containers; columns are a
  sortable row. `onDragEnd` computes the new order/position and fires the
  corresponding mutation (`MoveCard` or `ReorderColumns`); the optimistic
  cache update happens in the mutation, not in local component state, so a
  WebSocket echo of the same change doesn't double-move anything.

## Routes and pages

- `/` — **BoardListPage**: replaces the current placeholder `HomePage` in
  `router.tsx`. Grid of the user's boards (`GET /boards`), each entry
  navigates to `/boards/:boardId`. "New board" control (inline form or
  small modal, title only) creates via mutation and navigates into the
  new board. Empty state: "No boards yet — create your first one." Loading
  state: skeleton grid.
- `/boards/:boardId` — **BoardPage**: kanban view. Header shows the board
  title (inline-editable, owner only for now — no per-field role gating
  beyond what the backend already enforces) and a "Members" button opening
  the members panel. Body: horizontally-scrolling row of columns.
- No separate route for members — a modal/panel overlaid on `BoardPage`.
- Both new routes are children of the existing `RequireAuth` + `AppLayout`
  wrapper in `router.tsx`, same as the current `/`.

## Kanban view components

- **Column** (`frontend/src/features/board/Column.tsx` or similar):
  inline-editable title, vertical list of cards, "add card" affordance at
  the bottom, a `⋯` menu for rename/delete. New "add column" affordance at
  the end of the column row.
- **Card** (`frontend/src/features/board/Card.tsx`): title, truncated
  description, assignee/due-date badges when present. Click opens a detail
  modal (title, description, assignee, due date — editable) with a delete
  action.
- Drag handles are the column header (for columns) and the card body (for
  cards); dropping a card on a different column's list reassigns it.

## Members panel

- Lists members (name, role). Owner sees an email input + "Invite" button
  and a remove control per non-owner member. Non-owners see a read-only
  list. Owner cannot remove themselves (backend already rejects this;
  frontend disables/hides the control for the owner row as a UX nicety,
  not a security boundary).

## Errors, loading, and empty states

- Initial fetch failures (board list, column list) show an inline message
  with a "Retry" button rather than a blank screen.
- Mutation failures (create/rename/delete/move/reorder/invite/remove) show
  a toast/inline error and roll back any optimistic update.
- Empty column shows just its "add card" affordance — no separate empty
  illustration needed.
- WebSocket disconnects reconnect silently (see Architecture); no
  user-facing error state for them.

## Testing

Same conventions as prior frontend phases: Vitest + Testing Library,
behavior-focused (not implementation-detail) tests. Mock `api/*.ts` modules
and the WebSocket (a small fake `WebSocket`/event-dispatch helper) rather
than dnd-kit internals. Cover: board list fetch/render/create, column/card
CRUD round-trips through the mocked API, a simulated drag-end firing the
correct mutation with the correct payload, and a simulated WebSocket event
updating the rendered UI without a refetch.

## Backend task: WebSocket Origin hardening

`internal/realtime/handler.go`'s `ServeWS` currently accepts any origin
(`websocket.AcceptOptions{InsecureSkipVerify: true}`), with a comment
noting this was acceptable only because no frontend existed yet. Now that
one does:

- `realtime.NewHandler` takes an additional `allowedOrigin string`
  parameter (reusing the same value already wired as
  `cfg.CORSAllowedOrigin` for the HTTP CORS middleware in
  `cmd/api/main.go`).
- `ServeWS` passes `websocket.AcceptOptions{OriginPatterns: []string{h.allowedOrigin}}`
  instead of `InsecureSkipVerify: true`.
- Remove the now-outdated comment explaining the previous skip.
- Add a handler test asserting a handshake from a disallowed `Origin`
  header is rejected, alongside the existing happy-path test.

## Out of scope

- Board/column/card CRUD on the backend — already implemented.
- Any change to the CORS HTTP middleware itself beyond reusing its existing
  config value for the WebSocket origin check.
- Search, filtering, labels/tags, activity feed, notifications — not part
  of the current backend either.
- Mobile-specific drag-and-drop touch tuning beyond whatever dnd-kit gives
  by default.
