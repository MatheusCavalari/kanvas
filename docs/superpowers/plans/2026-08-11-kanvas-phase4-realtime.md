# Kanvas — Phase 4: Realtime (WebSocket) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Live collaboration — when one client mutates a board's columns or cards via the REST API, every other client currently viewing that board sees the change over a WebSocket connection, without polling.

**Architecture:** A new `internal/realtime` package: an in-process `Hub` that fans out board-scoped events to subscriber channels (no external message broker — a single-process deployment is the explicit Phase 1 scope, so this is deliberately simple), plus a `Handler` exposing `GET /boards/{boardID}/ws`. `internal/card`'s service layer gets a small `EventPublisher` port (same decoupling shape as `BoardAuthorizer`/`UserLookup` from Phases 2-3) and calls `Publish` after each successful mutation; `*realtime.Hub` satisfies that interface structurally. `internal/realtime` itself depends on nothing from `card` or `board` — it only needs a `TokenParser` (satisfied by `*jwt.Issuer`) and a `BoardAuthorizer` (satisfied by `*board.Service`) to authenticate and authorize the WebSocket handshake, mirroring the exact interfaces `card` already defined for the same purposes.

**Tech Stack:** Go 1.23, [`github.com/coder/websocket`](https://github.com/coder/websocket) (the actively maintained successor to `nhooyr.io/websocket`, referenced in the original design spec) for the WebSocket transport — the only new dependency this phase adds.

## Global Constraints

- Go module path: `github.com/MatheusCavalari/kanvas/backend`. Go version: **1.23** — after `go get github.com/coder/websocket`, run `go mod edit -go=1.23` then `go mod tidy`, check `head -3 backend/go.mod`. If the newest `coder/websocket` release requires a newer Go version, pin to an older compatible release the way earlier phases pinned pgx/testify/testcontainers — do not accept a go.mod bump.
- Event types are exactly the eight named in the design spec, no more: `column.created`, `column.updated`, `column.deleted`, `column.reordered`, `card.created`, `card.updated`, `card.deleted`, `card.moved`. Incidental side-effect renumbering (e.g. the sibling reorder that already happens after a delete, per Phase 3's final-review fix) does **not** get its own event — the primary event (`column.deleted`/`card.deleted`) is the only signal; a client should treat it as "refetch this board's columns" rather than expect a matching reorder event for the cleanup.
- The WebSocket handshake is authenticated differently from every other endpoint: browsers cannot set custom headers on a `WebSocket` handshake, so the access token travels as a query parameter (`?token=<access_token>`), validated inside the handler itself — `GET /boards/{boardID}/ws` is **not** wrapped in the shared `authMiddleware`.
- `websocket.Accept` is called with `InsecureSkipVerify: true` (disables the browser-Origin check) — acceptable for now since there is no frontend yet (Phases 5+) and no cookie-based ambient credential on this endpoint (the token is an explicit query parameter). This is a documented, deliberate limitation to revisit once the frontend's origin is known — not an oversight.
- The Hub is in-process, in-memory only: subscriber state is lost on server restart (clients reconnect and refetch), and there is exactly one hub per process — this does not scale past a single backend instance. That's explicitly out of scope; a later phase would swap in Postgres `LISTEN`/`NOTIFY` or Redis pub/sub if multi-instance deployment is ever needed.
- `internal/card`'s `Service.MoveCard` already carries a documented comment (added in Phase 3's final-review fix) about its non-transactional lost-update risk under concurrent moves — do not touch that comment or attempt to fix it in this phase; realtime broadcast makes that race more *visible* (multiple clients watching), not more *likely*.
- Error envelope and route-registration conventions (chi router, `RegisterRoutes(r chi.Router, ...)`) follow the same pattern as `auth`/`board`/`card`, except `realtime.Handler.RegisterRoutes` takes no `authMiddleware` parameter (see the auth-differs-here constraint above).
- Integration tests are gated behind `//go:build integration` and use `internal/platform/db/dbtest.NewPool(t)`, unchanged from earlier phases.

---

## Task Overview

1. `internal/realtime` Hub — in-process event fan-out (no HTTP, no new dependency)
2. Wire `EventPublisher` into `internal/card`'s service layer (retrofit Phase 3)
3. `internal/realtime` WebSocket handler — auth, upgrade, broadcast loop (adds the `coder/websocket` dependency)
4. Wire `main.go` + end-to-end integration test (real WebSocket client observes a REST mutation) + README update

---

### Task 1: `internal/realtime` Hub

**Files:**
- Create: `backend/internal/realtime/hub.go`
- Create: `backend/internal/realtime/hub_test.go`

**Interfaces:**
- Produces: `realtime.Event{Type, BoardID, Data}`, `realtime.NewHub() *Hub`, `(*Hub).Publish(ctx, boardID uuid.UUID, eventType string, payload interface{})`, `(*Hub).SubscriberCount(boardID uuid.UUID) int` — consumed by Task 2 (as the concrete `EventPublisher`) and Task 3 (same package, uses the unexported `subscribe`/`unsubscribe`).

This task has **no new dependency** — pure Go (`context`, `sync`, `uuid`). The WebSocket library is added in Task 3, not here, so a mistake in this task can't be blamed on an unfamiliar library.

- [ ] **Step 1: Write the failing tests**

`backend/internal/realtime/hub_test.go`:
```go
package realtime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHub_PublishDeliversToSubscriber(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)
	require.Equal(t, 1, hub.SubscriberCount(boardID))

	hub.Publish(ctx, boardID, "card.created", map[string]string{"id": "abc"})

	select {
	case event := <-ch:
		require.Equal(t, "card.created", event.Type)
		require.Equal(t, boardID, event.BoardID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_PublishOnlyReachesSubscribersOfThatBoard(t *testing.T) {
	hub := NewHub()
	boardA := uuid.New()
	boardB := uuid.New()
	ctx := context.Background()

	chA := hub.subscribe(boardA)
	defer hub.unsubscribe(boardA, chA)
	chB := hub.subscribe(boardB)
	defer hub.unsubscribe(boardB, chB)

	hub.Publish(ctx, boardA, "card.created", nil)

	select {
	case <-chA:
	case <-time.After(time.Second):
		t.Fatal("boardA subscriber should have received the event")
	}

	select {
	case <-chB:
		t.Fatal("boardB subscriber should not have received boardA's event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	hub.unsubscribe(boardID, ch)
	require.Equal(t, 0, hub.SubscriberCount(boardID))

	hub.Publish(ctx, boardID, "card.created", nil)

	_, ok := <-ch
	require.False(t, ok, "channel should be closed after unsubscribe")
}

func TestHub_PublishDoesNotBlockOnFullSubscriberBuffer(t *testing.T) {
	hub := NewHub()
	boardID := uuid.New()
	ctx := context.Background()

	ch := hub.subscribe(boardID)
	defer hub.unsubscribe(boardID, ch)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			hub.Publish(ctx, boardID, "card.created", i)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish should never block, even with an unread subscriber buffer")
	}
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/realtime/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Implement the hub**

`backend/internal/realtime/hub.go`:
```go
package realtime

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// Event is the envelope broadcast to WebSocket subscribers of a board.
type Event struct {
	Type    string      `json:"type"`
	BoardID uuid.UUID   `json:"board_id"`
	Data    interface{} `json:"data"`
}

// Hub fans out board-scoped events to any number of subscribers. It has
// no knowledge of HTTP or WebSockets — internal/card depends on it
// structurally, via its own EventPublisher interface, purely as
// something with a Publish method; the WebSocket transport lives in
// handler.go, same package, added in Task 3.
type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan Event]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[uuid.UUID]map[chan Event]bool)}
}

// subscribe registers a new subscriber channel for boardID. The returned
// channel is buffered so a slow reader doesn't block Publish; if the
// buffer fills, Publish drops the event for that subscriber rather than
// blocking every other subscriber and every REST request that publishes.
func (h *Hub) subscribe(boardID uuid.UUID) chan Event {
	ch := make(chan Event, 16)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[boardID] == nil {
		h.clients[boardID] = make(map[chan Event]bool)
	}
	h.clients[boardID][ch] = true
	return ch
}

func (h *Hub) unsubscribe(boardID uuid.UUID, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[boardID], ch)
	if len(h.clients[boardID]) == 0 {
		delete(h.clients, boardID)
	}
	close(ch)
}

// Publish broadcasts an event to every current subscriber of boardID. It
// never blocks on a slow subscriber and is safe to call concurrently.
func (h *Hub) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	event := Event{Type: eventType, BoardID: boardID, Data: payload}
	for ch := range h.clients[boardID] {
		select {
		case ch <- event:
		default:
		}
	}
}

// SubscriberCount reports how many active subscribers boardID currently
// has. Exported for tests and for a future health/metrics endpoint.
func (h *Hub) SubscriberCount(boardID uuid.UUID) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[boardID])
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/realtime/... -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/realtime
git commit -m "feat(backend): add realtime Hub for board-scoped event fan-out"
```

---

### Task 2: Wire `EventPublisher` into `internal/card`'s service layer

**Files:**
- Modify: `backend/internal/card/repository.go` (add the `EventPublisher` interface)
- Modify: `backend/internal/card/service.go` (add event constants, payload types, `events EventPublisher` field, and a `Publish` call after every successful mutation)
- Modify: `backend/internal/card/service_test.go` (every existing `NewService(repo, boardAuth)` call site becomes `NewService(repo, boardAuth, newFakeEventPublisher())`, plus 3 new tests)
- Modify: `backend/internal/card/repository_fake_test.go` (add `fakeEventPublisher`)

**Interfaces:**
- Produces: `card.EventPublisher` interface (`Publish(ctx, boardID uuid.UUID, eventType string, payload interface{})`), `card.NewService(repo Repository, board BoardAuthorizer, events EventPublisher) *Service` (signature change — every existing caller must be updated), event type constants (`card.EventColumnCreated`, etc.) — consumed by Task 4 (`*realtime.Hub` satisfies `EventPublisher` structurally, wired in `main.go`).

This is a retrofit of an already-merged file — the steps below give the **complete final content** of `service.go` so there's no ambiguity, since this task changes nearly every method.

- [ ] **Step 1: Add the `EventPublisher` interface**

Append to `backend/internal/card/repository.go` (after the existing `BoardAuthorizer` interface, same file):
```go

// EventPublisher broadcasts a board-scoped realtime event. Implemented
// by *realtime.Hub — card's service layer never imports
// internal/realtime directly, the same decoupling pattern as
// BoardAuthorizer.
type EventPublisher interface {
	Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{})
}
```

- [ ] **Step 2: Write the failing tests**

Append to `backend/internal/card/service_test.go` (add `newFakeEventPublisher` usage — the type itself comes in Step 5 below):
```go
func TestService_CreateColumn_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	_, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventColumnCreated, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}

func TestService_DeleteCard_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)
	events.reset()

	err = svc.DeleteCard(ctx, card.ID, member)
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventCardDeleted, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}

func TestService_MoveCard_PublishesEvent(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	events := newFakeEventPublisher()
	svc := NewService(repo, boardAuth, events)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)
	events.reset()

	_, err = svc.MoveCard(ctx, card.ID, member, column.ID, 0)
	require.NoError(t, err)

	require.Len(t, events.events, 1)
	require.Equal(t, EventCardMoved, events.events[0].eventType)
	require.Equal(t, boardID, events.events[0].boardID)
}
```

Then update every one of the 23 existing `svc := NewService(repo, boardAuth)` lines already in this file to `svc := NewService(repo, boardAuth, newFakeEventPublisher())` — a uniform find-and-replace, since every occurrence is textually identical.

- [ ] **Step 3: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/card/... -v`
Expected: FAIL — `NewService` called with 2 args doesn't match the new 3-arg signature yet (compile error), and `newFakeEventPublisher`/`EventColumnCreated`/etc. are undefined.

- [ ] **Step 4: Replace `service.go` with its updated content**

`backend/internal/card/service.go` (full file):
```go
package card

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	EventColumnCreated   = "column.created"
	EventColumnUpdated   = "column.updated"
	EventColumnDeleted   = "column.deleted"
	EventColumnReordered = "column.reordered"
	EventCardCreated     = "card.created"
	EventCardUpdated     = "card.updated"
	EventCardDeleted     = "card.deleted"
	EventCardMoved       = "card.moved"
)

type columnDeletedEvent struct {
	ID      uuid.UUID `json:"id"`
	BoardID uuid.UUID `json:"board_id"`
}

type cardDeletedEvent struct {
	ID       uuid.UUID `json:"id"`
	ColumnID uuid.UUID `json:"column_id"`
}

type columnsReorderedEvent struct {
	BoardID   uuid.UUID   `json:"board_id"`
	ColumnIDs []uuid.UUID `json:"column_ids"`
}

type Service struct {
	repo   Repository
	board  BoardAuthorizer
	events EventPublisher
}

func NewService(repo Repository, board BoardAuthorizer, events EventPublisher) *Service {
	return &Service{repo: repo, board: board, events: events}
}

func (s *Service) CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Column{}, err
	}
	column, err := s.repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: title})
	if err != nil {
		return Column{}, err
	}
	s.events.Publish(ctx, boardID, EventColumnCreated, toColumnView(column, nil))
	return column, nil
}

func (s *Service) RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Column{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Column{}, err
	}
	renamed, err := s.repo.RenameColumn(ctx, columnID, title)
	if err != nil {
		return Column{}, err
	}
	s.events.Publish(ctx, column.BoardID, EventColumnUpdated, toColumnView(renamed, nil))
	return renamed, nil
}

func (s *Service) DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return err
	}
	if err := s.repo.DeleteColumn(ctx, columnID); err != nil {
		return err
	}
	s.events.Publish(ctx, column.BoardID, EventColumnDeleted, columnDeletedEvent{ID: columnID, BoardID: column.BoardID})

	remaining, err := s.repo.ListColumnsByBoard(ctx, column.BoardID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(remaining))
	for _, c := range remaining {
		ids = append(ids, c.ID)
	}
	return s.repo.ReorderColumns(ctx, column.BoardID, ids)
}

func (s *Service) ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return err
	}
	current, err := s.repo.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return err
	}
	if !isExactPermutation(current, orderedColumnIDs) {
		return ErrInvalidReorder
	}
	if err := s.repo.ReorderColumns(ctx, boardID, orderedColumnIDs); err != nil {
		return err
	}
	s.events.Publish(ctx, boardID, EventColumnReordered, columnsReorderedEvent{BoardID: boardID, ColumnIDs: orderedColumnIDs})
	return nil
}

// isExactPermutation reports whether orderedIDs contains exactly the same
// set of column IDs as current, with no duplicates and no missing/extra
// entries.
func isExactPermutation(current []Column, orderedIDs []uuid.UUID) bool {
	if len(current) != len(orderedIDs) {
		return false
	}
	currentSet := make(map[uuid.UUID]struct{}, len(current))
	for _, c := range current {
		currentSet[c.ID] = struct{}{}
	}
	seen := make(map[uuid.UUID]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, ok := currentSet[id]; !ok {
			return false
		}
		if _, dup := seen[id]; dup {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}

type ColumnWithCards struct {
	Column
	Cards []Card
}

func (s *Service) ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}
	columns, err := s.repo.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	result := make([]ColumnWithCards, 0, len(columns))
	for _, column := range columns {
		cards, err := s.repo.ListCardsByColumn(ctx, column.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, ColumnWithCards{Column: column, Cards: cards})
	}
	return result, nil
}

func mapColumnErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrColumnNotFound
	}
	return err
}

func (s *Service) CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Card{}, err
	}
	created, err := s.repo.CreateCard(ctx, Card{
		ID:          uuid.New(),
		ColumnID:    columnID,
		Title:       title,
		Description: description,
		AssigneeID:  assigneeID,
		DueDate:     dueDate,
	})
	if err != nil {
		return Card{}, err
	}
	s.events.Publish(ctx, column.BoardID, EventCardCreated, toCardView(created))
	return created, nil
}

func (s *Service) UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return Card{}, mapCardErr(err)
	}
	column, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Card{}, err
	}
	existing.Title = title
	existing.Description = description
	existing.AssigneeID = assigneeID
	existing.DueDate = dueDate
	updated, err := s.repo.UpdateCard(ctx, existing)
	if err != nil {
		return Card{}, err
	}
	s.events.Publish(ctx, column.BoardID, EventCardUpdated, toCardView(updated))
	return updated, nil
}

func (s *Service) DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return mapCardErr(err)
	}
	column, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return err
	}
	if err := s.repo.DeleteCard(ctx, cardID); err != nil {
		return err
	}
	s.events.Publish(ctx, column.BoardID, EventCardDeleted, cardDeletedEvent{ID: cardID, ColumnID: column.ID})

	remaining, err := s.repo.ListCardsByColumn(ctx, column.ID)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(remaining))
	for _, c := range remaining {
		ids = append(ids, c.ID)
	}
	return s.repo.ReorderCards(ctx, column.ID, ids)
}

// MoveCard reads the target column's cards, computes the new order in Go,
// and writes it back. This is not protected by a transaction or version
// check, so two concurrent moves into the same column can race and produce
// duplicate positions (a lost update). A deterministic ORDER BY tiebreaker
// keeps list order stable and reproducible even if that happens, but a full
// transactional/locking fix is a known follow-up, not addressed here.
func (s *Service) MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error) {
	existing, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return Card{}, mapCardErr(err)
	}
	sourceColumn, err := s.repo.GetColumnByID(ctx, existing.ColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, sourceColumn.BoardID, requesterID); err != nil {
		return Card{}, err
	}

	targetColumn, err := s.repo.GetColumnByID(ctx, targetColumnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if targetColumn.BoardID != sourceColumn.BoardID {
		return Card{}, ErrColumnNotFound
	}

	movingToNewColumn := targetColumnID != existing.ColumnID
	if movingToNewColumn {
		if err := s.repo.SetCardColumn(ctx, cardID, targetColumnID); err != nil {
			return Card{}, err
		}
	}

	targetCards, err := s.repo.ListCardsByColumn(ctx, targetColumnID)
	if err != nil {
		return Card{}, err
	}
	orderedTargetIDs := reorderWithInsert(targetCards, cardID, targetPosition)
	if err := s.repo.ReorderCards(ctx, targetColumnID, orderedTargetIDs); err != nil {
		return Card{}, err
	}

	if movingToNewColumn {
		sourceCards, err := s.repo.ListCardsByColumn(ctx, sourceColumn.ID)
		if err != nil {
			return Card{}, err
		}
		orderedSourceIDs := make([]uuid.UUID, 0, len(sourceCards))
		for _, c := range sourceCards {
			orderedSourceIDs = append(orderedSourceIDs, c.ID)
		}
		if err := s.repo.ReorderCards(ctx, sourceColumn.ID, orderedSourceIDs); err != nil {
			return Card{}, err
		}
	}

	moved, err := s.repo.GetCardByID(ctx, cardID)
	if err != nil {
		return Card{}, err
	}
	s.events.Publish(ctx, sourceColumn.BoardID, EventCardMoved, toCardView(moved))
	return moved, nil
}

func mapCardErr(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrCardNotFound
	}
	return err
}

// reorderWithInsert returns the full ordered card-ID list for a column
// after moving/inserting movingCardID to targetPosition (clamped to the
// valid range), removing any existing occurrence of movingCardID first.
// Pure function — no I/O — so it's cheap to unit test directly.
func reorderWithInsert(cards []Card, movingCardID uuid.UUID, targetPosition int) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(cards))
	for _, c := range cards {
		if c.ID != movingCardID {
			ids = append(ids, c.ID)
		}
	}
	if targetPosition < 0 {
		targetPosition = 0
	}
	if targetPosition > len(ids) {
		targetPosition = len(ids)
	}
	result := make([]uuid.UUID, 0, len(ids)+1)
	result = append(result, ids[:targetPosition]...)
	result = append(result, movingCardID)
	result = append(result, ids[targetPosition:]...)
	return result
}
```

(The only substantive changes from the file's current content: the `events`-related constants/types near the top, `Service.events` field, `NewService`'s third parameter, and one `s.events.Publish(...)` call inserted right after each mutation succeeds, right before that method's success return. `isExactPermutation`, `mapColumnErr`, `mapCardErr`, and `reorderWithInsert` are unchanged — reproduced here only because this is a full-file replacement.)

- [ ] **Step 5: Add the fake event publisher**

Append to `backend/internal/card/repository_fake_test.go`:
```go

type publishedEvent struct {
	boardID   uuid.UUID
	eventType string
	payload   interface{}
}

type fakeEventPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

func newFakeEventPublisher() *fakeEventPublisher {
	return &fakeEventPublisher{}
}

func (f *fakeEventPublisher) Publish(ctx context.Context, boardID uuid.UUID, eventType string, payload interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, publishedEvent{boardID: boardID, eventType: eventType, payload: payload})
}

func (f *fakeEventPublisher) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = nil
}
```
(`sync` is already imported in this file for `fakeRepository`'s mutex — no new import needed.)

- [ ] **Step 6: Run the tests and confirm they pass**

Run (from `backend/`): `go build ./... && go test ./internal/card/... -v`
Expected: PASS (all existing tests plus the 3 new ones)

- [ ] **Step 7: Verify the rest of the module still builds**

Run (from `backend/`): `go build ./...`
Expected: FAILS at this point — `cmd/api/main.go` still calls the old 2-arg `card.NewService(cardRepo, boardService)`. That's expected and will be fixed in Task 4; do not fix `main.go` in this task. Confirm the failure is exactly that one call site (`cmd/api/main.go`) and nothing else, then proceed to commit — `go build ./internal/...` (scoped to packages, excluding `cmd/api`) should succeed cleanly if you want a clean-build confirmation before committing.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/card
git commit -m "feat(backend): publish realtime events from card service mutations"
```

---

### Task 3: `internal/realtime` WebSocket handler

**Files:**
- Create: `backend/internal/realtime/handler.go`
- Create: `backend/internal/realtime/handler_test.go`

**Interfaces:**
- Consumes: `realtime.Hub` (Task 1, same package — uses the unexported `subscribe`/`unsubscribe`).
- Produces: `realtime.TokenParser` interface (`ParseAccessToken(token string) (uuid.UUID, error)` — satisfied by `*jwt.Issuer`), `realtime.BoardAuthorizer` interface (`EnsureMember(ctx, boardID, userID uuid.UUID) error` — satisfied by `*board.Service`), `realtime.NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer) *Handler`, `(*Handler).RegisterRoutes(r chi.Router)` mounting `GET /boards/{boardID}/ws` — consumed by Task 4.

- [ ] **Step 1: Add the `coder/websocket` dependency**

Run (from `backend/`):
```bash
go get github.com/coder/websocket@latest
```
Then immediately: `go mod edit -go=1.23 && go mod tidy`, and confirm `head -3 backend/go.mod` shows `go 1.23`. If tidy fails or re-bumps the directive because the latest release requires a newer Go version, pin to an older release (`go get github.com/coder/websocket@v1.8.12` or similar — check what's available) instead of accepting the bump.

- [ ] **Step 2: Write the failing tests**

`backend/internal/realtime/handler_test.go`:
```go
package realtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeTokenParser struct {
	userID uuid.UUID
	err    error
}

func (f *fakeTokenParser) ParseAccessToken(token string) (uuid.UUID, error) {
	if f.err != nil {
		return uuid.UUID{}, f.err
	}
	return f.userID, nil
}

type fakeWSBoardAuthorizer struct {
	allow bool
}

func (f *fakeWSBoardAuthorizer) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	if f.allow {
		return nil
	}
	return errors.New("forbidden")
}

func TestHandler_ServeWS_MissingToken(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub, &fakeTokenParser{userID: uuid.New()}, &fakeWSBoardAuthorizer{allow: true})
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/boards/" + uuid.New().String() + "/ws")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestHandler_ServeWS_NotAMember(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub, &fakeTokenParser{userID: uuid.New()}, &fakeWSBoardAuthorizer{allow: false})
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/boards/" + uuid.New().String() + "/ws?token=whatever")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestHandler_ServeWS_DeliversPublishedEvent(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()
	boardID := uuid.New()
	h := NewHandler(hub, &fakeTokenParser{userID: userID}, &fakeWSBoardAuthorizer{allow: true})
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/boards/" + boardID.String() + "/ws?token=whatever"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardID) == 1
	}, time.Second, 10*time.Millisecond)

	hub.Publish(context.Background(), boardID, "card.created", map[string]string{"id": "abc"})

	var received Event
	require.NoError(t, wsjson.Read(ctx, conn, &received))
	require.Equal(t, "card.created", received.Type)
	require.Equal(t, boardID, received.BoardID)

	conn.Close(websocket.StatusNormalClosure, "")
}
```

- [ ] **Step 3: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/realtime/... -run TestHandler -v`
Expected: FAIL — `NewHandler` undefined.

- [ ] **Step 4: Implement the handler**

`backend/internal/realtime/handler.go`:
```go
package realtime

import (
	"context"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// TokenParser validates a JWT access token and returns the user ID it
// was issued to. Implemented by *jwt.Issuer — this package never imports
// internal/platform/jwt directly, same decoupling pattern used elsewhere.
type TokenParser interface {
	ParseAccessToken(token string) (uuid.UUID, error)
}

// BoardAuthorizer checks board membership. Implemented by *board.Service,
// the same interface shape internal/card already depends on.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}

type Handler struct {
	hub    *Hub
	tokens TokenParser
	board  BoardAuthorizer
}

func NewHandler(hub *Hub, tokens TokenParser, board BoardAuthorizer) *Handler {
	return &Handler{hub: hub, tokens: tokens, board: board}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	// Deliberately NOT behind the shared authMiddleware: browsers cannot
	// set custom headers on a WebSocket handshake, so the access token
	// travels as a query parameter instead and is validated here.
	r.Get("/boards/{boardID}/ws", h.ServeWS)
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	boardID, err := uuid.Parse(chi.URLParam(r, "boardID"))
	if err != nil {
		http.Error(w, "invalid board id", http.StatusBadRequest)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	userID, err := h.tokens.ParseAccessToken(token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	if err := h.board.EnsureMember(r.Context(), boardID, userID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// InsecureSkipVerify disables the Origin check. Acceptable for now:
	// there is no browser frontend yet (Phase 5+) and no cookie-based
	// auth on this endpoint (the token is an explicit query parameter,
	// not an ambient credential), so there's no CSRF-style risk this
	// check would prevent. Revisit once the frontend's origin is known
	// and replace with an explicit OriginPatterns allowlist.
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	ctx := conn.CloseRead(r.Context())

	events := h.hub.subscribe(boardID)
	defer h.hub.unsubscribe(boardID, events)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := wsjson.Write(writeCtx, conn, event)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
```

- [ ] **Step 5: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/realtime/... -v`
Expected: PASS (all `realtime` package tests, including the 3 new handler tests)

- [ ] **Step 6: Commit**

```bash
git add backend/internal/realtime backend/go.mod backend/go.sum
git commit -m "feat(backend): add realtime WebSocket handler with query-token auth"
```

---

### Task 4: Wire `main.go` + end-to-end integration test + README update

**Files:**
- Modify: `backend/cmd/api/main.go`
- Create: `backend/internal/realtime/e2e_test.go` (build tag `integration`, package `realtime_test`)
- Modify: `backend/README.md`

**Interfaces:**
- Consumes: everything produced by Tasks 1-3, plus the existing `auth`/`board`/`card` wiring.
- Produces: a fully wired running server where REST mutations broadcast live over `/boards/{boardID}/ws` — this is what Phase 5+ (the frontend) will connect to.

- [ ] **Step 1: Write the failing end-to-end test**

`backend/internal/realtime/e2e_test.go`:
```go
//go:build integration

package realtime_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func TestRealtimeFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)
	authMiddleware := middleware.Auth(issuer)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()
	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, hub)
	cardHandler := card.NewHandler(cardService)
	realtimeHandler := realtime.NewHandler(hub, issuer, boardService)

	router := httpserver.NewRouter()
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	realtimeHandler.RegisterRoutes(router)

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBoardBody, _ := json.Marshal(map[string]string{"name": "Sprint Board"})
	createBoardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/", bytes.NewReader(createBoardBody))
	createBoardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createBoardResp, err := client.Do(createBoardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createBoardResp.StatusCode)
	var boardCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createBoardResp.Body).Decode(&boardCreated))
	_ = createBoardResp.Body.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/boards/" + boardCreated.ID + "/ws?token=" + ownerToken
	dialCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	boardUUID, err := uuid.Parse(boardCreated.ID)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return hub.SubscriberCount(boardUUID) == 1
	}, 2*time.Second, 10*time.Millisecond)

	createColumnBody, _ := json.Marshal(map[string]string{"title": "To Do"})
	createColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(createColumnBody))
	createColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createColumnResp, err := client.Do(createColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createColumnResp.StatusCode)
	_ = createColumnResp.Body.Close()

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	var event realtime.Event
	require.NoError(t, wsjson.Read(readCtx, conn, &event))
	require.Equal(t, "column.created", event.Type)
	require.Equal(t, boardUUID, event.BoardID)

	conn.Close(websocket.StatusNormalClosure, "")
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `backend/`): `make test-integration`
Expected: this test doesn't depend on `main.go`, so it should compile and exercise real code from Tasks 1-3; read any failure carefully — it should point at real logic, not a missing symbol.

- [ ] **Step 3: Wire `main.go`**

`backend/cmd/api/main.go` (full file):
```go
package main

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/MatheusCavalari/kanvas/backend/internal/auth"
	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/config"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
	"github.com/MatheusCavalari/kanvas/backend/internal/realtime"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	if err := runMigrations(cfg.DatabaseURL, cfg.MigrationsPath); err != nil {
		log.Fatalf("running migrations: %v", err)
	}

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connecting to database: %v", err)
	}
	defer pool.Close()

	queries := gen.New(pool)
	issuer := jwt.NewIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)
	authMiddleware := middleware.Auth(issuer)

	authRepo := auth.NewPostgresRepository(queries)
	authService := auth.NewService(authRepo, issuer, cfg.RefreshTokenTTL)
	authHandler := auth.NewHandler(authService, cfg.SecureCookies)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	hub := realtime.NewHub()

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService, hub)
	cardHandler := card.NewHandler(cardService)

	realtimeHandler := realtime.NewHandler(hub, issuer, boardService)

	router := httpserver.NewRouter()
	authHandler.RegisterRoutes(router, authMiddleware)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)
	realtimeHandler.RegisterRoutes(router)

	log.Printf("listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runMigrations applies all pending SQL migrations from migrationsPath
// against databaseURL. It is idempotent — running it against an
// already-up-to-date database is a no-op.
func runMigrations(databaseURL, migrationsPath string) error {
	m, err := migrate.New("file://"+migrationsPath, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = m.Close()
	}()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
```

(New versus the current file: the `realtime` import, the `hub := realtime.NewHub()` line, `cardService`'s third argument (`hub`), the `realtimeHandler` construction, and its `RegisterRoutes(router)` call — note no `authMiddleware` argument, per this phase's auth-differs-here constraint.)

- [ ] **Step 4: Run the integration test and confirm it passes**

Run (from `backend/`): `make test-integration`
Expected: PASS

- [ ] **Step 5: Run the full suite and smoke-test manually**

Run (from `backend/`): `go build ./... && go test ./... -race && go test ./... -race -tags=integration`
Expected: all green.

Manual smoke test (with `docker compose up -d postgres` running): `make run` in one terminal, then in another, register a user and create a board via `curl` exactly as in earlier phases, confirming both still return success (this proves the REST surface didn't regress). Full manual verification of the WebSocket itself is covered by the automated e2e test above — connecting a real browser or `websocat` client is optional and not required for this task, since Phase 5+ will be the first real WebSocket client. Stop the server with Ctrl+C when done.

- [ ] **Step 6: Update `backend/README.md`**

Add a new section after "Columns & cards", and add `internal/realtime/` to "Project layout":
```markdown
## Realtime (WebSocket)

    GET /boards/{boardID}/ws?token=<access_token>

Unlike every other endpoint, the access token is a query parameter, not an `Authorization` header — browsers can't set custom headers on a WebSocket handshake. The connection is authenticated and board-membership-checked before the upgrade; a missing/invalid token gets `401`, a valid token for a non-member gets `403`.

Once connected, the client receives one JSON message per board event, no polling needed:

    {"type": "card.created", "board_id": "...", "data": { ...card fields, same shape as the REST response... }}

Event types: `column.created`, `column.updated`, `column.deleted`, `column.reordered`, `card.created`, `card.updated`, `card.deleted`, `card.moved`. A `*.deleted` event's `data` is just `{"id": "...", ...parent_id}` (the resource is gone); treat it as a signal to refetch that board's columns rather than expecting a separate reorder event for any cleanup renumbering that happened alongside the delete.

The hub is in-process and in-memory: it does not survive a restart and does not work across multiple backend instances — fine for this project's single-instance deployment target, not something to build a multi-instance production system on without swapping in a real pub/sub backend first.
```

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/api/main.go backend/internal/realtime/e2e_test.go backend/README.md
git commit -m "feat(backend): wire realtime WebSocket into main.go and add end-to-end broadcast test"
```

---

## Definition of Done

- `make test` and `make test-integration` both pass locally.
- A fresh `docker compose down -v && docker compose up -d --build` followed by register → create board → connect WebSocket → create a column via `curl` → observe the `column.created` event arrive on the WebSocket, all work end to end (the automated e2e test in Task 4 covers this over `httptest`; a full Docker-Compose-based manual pass is optional but recommended before calling the phase done).
- `make lint` is clean.
- GitHub Actions is green on the pushed branch (the existing `backend-ci.yml` workflow needs no changes — it already runs `go test ./...` and `-tags=integration` across every package, including the new `internal/realtime`).
