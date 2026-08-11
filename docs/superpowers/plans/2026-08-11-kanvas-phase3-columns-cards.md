# Kanvas — Phase 3: Columns & Cards — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working, tested REST API for the actual Kanban mechanics: columns within a board, cards within a column, drag-and-drop-style reordering within and across columns, all gated by board membership.

**Architecture:** A new `internal/card` package (this phase's domain package name follows the design spec's original three-package split: `auth`, `board`, `card`) covering BOTH columns and cards — they're one cohesive "board contents" concern, tightly coupled (a card's authorization always resolves through its column's board). Same Clean/Hexagonal shape as `auth` and `board`: domain/repository/service/handler. Authorization reuses `board`'s existing membership check via a small exported method (`board.Service.EnsureMember`, added in Task 3) and a `BoardAuthorizer` interface defined in `card` — the same decoupling pattern Phase 2 used for `UserLookup`. `card`'s handler layer references `board`'s exported error sentinels directly for HTTP status mapping (a one-directional, read-only dependency — `board` never imports `card`).

**Tech Stack:** Same as Phases 1-2 — Go 1.23, chi, sqlc + pgx v5, golang-migrate, testcontainers-go for integration tests, testify.

## Global Constraints

- Go module path: `github.com/MatheusCavalari/kanvas/backend`. Go version: **1.23** — after any `go get`, run `go mod edit -go=1.23` then `go mod tidy`, check `head -3 backend/go.mod`. This phase adds **no new third-party dependencies**.
- sqlc regeneration MUST go through Docker: `make sqlc-generate` (already wired) or `MSYS_NO_PATHCONV=1 docker run --rm -v "$(pwd):/src" -w //src sqlc/sqlc:1.27.0 generate` from `backend/`. Never hand-write generated files.
- Error envelope: `{"error": {"code": "...", "message": "..."}}` via a per-package `writeError`/`writeJSON` helper pair (same duplicated-on-purpose pattern as `auth` and `board`).
- Migrations continue the existing numbering: `000005` (columns), `000006` (cards).
- Positions are `INTEGER` columns, zero-based, dense (no gaps) within their parent (columns within a board, cards within a column). Every mutation that changes membership or order of a position-having collection **renumbers the whole collection in one bulk SQL statement** (via `unnest(...)` + `UPDATE ... FROM`), not per-row updates — this keeps each reorder atomic without needing explicit transaction plumbing, the same one-CTE-statement discipline Phase 2 established for atomic board creation.
- Card `assignee_id` is nullable and FK's to `users(id) ON DELETE SET NULL` — assigning a nonexistent user ID must be translated from a raw FK-violation error to a clean domain error (`ErrAssigneeNotFound`), the same way Phase 1/2 translate unique-violations. Assigning a user who isn't a board member is NOT validated in this phase (explicit scope cut — YAGNI; can be added later without a migration).
- `card`'s `BoardAuthorizer` interface is satisfied by `*board.Service` (via its new exported `EnsureMember` method) — `card`'s **service** layer never imports `board` directly, only depends on the interface. `card`'s **handler** layer DOES import `board` for its two exported error sentinels (`board.ErrNotAMember`, `board.ErrForbidden`) purely for HTTP status-code translation — this is a deliberate, one-directional, read-only exception to the "packages don't import siblings" habit, scoped to error-constant references only.
- Per the design spec's already-documented v1 limitation: concurrent reorders from two clients can race (last write wins); this phase does not add optimistic locking. Not a regression to fix here.
- Integration tests are gated behind `//go:build integration` and use `internal/platform/db/dbtest.NewPool(t)` (already applies every migration under `backend/db/migrations`, including this phase's new ones).
- `internal/platform/httpserver.NewRouter()` still takes no arguments. `main.go` calls it once, then calls `RegisterRoutes(router, authMiddleware)` for auth, board, and now card handlers in turn — chi merges the overlapping `/boards/{boardID}/...` route trees from `board` and `card` correctly as long as the path parameter name (`boardID`) matches, which it does.

---

## Task Overview

1. Migrations: `columns`, `cards`
2. sqlc queries: columns, cards (+ a new nullable-`uuid` sqlc.yaml override for `assignee_id`) + regenerate
3. `board.Service.EnsureMember` export + `card` domain types + repository interface + `BoardAuthorizer` interface + test fakes
4. Card-package service: column operations (Create/Rename/Delete/Reorder) + `ListBoardColumns` (columns with nested cards — the main board-view read)
5. Card-package service: card operations (Create/Update/Delete/Move) + the `reorderWithInsert` pure helper
6. Postgres repository (integration-tested)
7. HTTP handlers + routes
8. Wire `main.go` + end-to-end integration test + README update

---

### Task 1: Migrations — `columns`, `cards`

**Files:**
- Create: `backend/db/migrations/000005_create_columns_table.up.sql`
- Create: `backend/db/migrations/000005_create_columns_table.down.sql`
- Create: `backend/db/migrations/000006_create_cards_table.up.sql`
- Create: `backend/db/migrations/000006_create_cards_table.down.sql`

**Interfaces:**
- Produces: `columns` table (`id uuid pk`, `board_id fk -> boards(id) ON DELETE CASCADE`, `title`, `position int`, `created_at`, `updated_at`, index on `board_id`) and `cards` table (`id uuid pk`, `column_id fk -> columns(id) ON DELETE CASCADE`, `title`, `description` default `''`, `position int`, `assignee_id uuid fk -> users(id) ON DELETE SET NULL`, `due_date timestamptz nullable`, `created_at`, `updated_at`, index on `column_id`) — consumed by Task 2's sqlc queries and Task 6's repository.

No Go code — verified by running the migrations against the local Postgres.

- [ ] **Step 1: Create the `columns` table migration**

`backend/db/migrations/000005_create_columns_table.up.sql`:
```sql
CREATE TABLE columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    position INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_columns_board_id ON columns(board_id);
```

`backend/db/migrations/000005_create_columns_table.down.sql`:
```sql
DROP TABLE IF EXISTS columns;
```

- [ ] **Step 2: Create the `cards` table migration**

`backend/db/migrations/000006_create_cards_table.up.sql`:
```sql
CREATE TABLE cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL,
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cards_column_id ON cards(column_id);
```

`backend/db/migrations/000006_create_cards_table.down.sql`:
```sql
DROP TABLE IF EXISTS cards;
```

- [ ] **Step 3: Run the migrations against the local Postgres**

Ensure Postgres is running (`docker compose up -d postgres` from the repo root if needed), then run (from `backend/`):
```bash
make migrate-up
```
Expected: output ending with `000006/u create_cards_table (...)`, no errors.

- [ ] **Step 4: Verify the tables exist**

Run (from repo root):
```bash
docker compose exec postgres psql -U kanvas -d kanvas -c "\dt"
```
Expected: lists `users`, `refresh_tokens`, `boards`, `board_members`, `columns`, `cards`, `schema_migrations`.

- [ ] **Step 5: Commit**

```bash
git add backend/db/migrations
git commit -m "feat(backend): add columns and cards migrations"
```

---

### Task 2: sqlc queries — columns, cards

**Files:**
- Create: `backend/db/queries/columns.sql`
- Create: `backend/db/queries/cards.sql`
- Modify: `backend/sqlc.yaml` (add a nullable-`uuid` override for `assignee_id`)
- Modify: `backend/internal/platform/db/gen/*.go` (regenerated by sqlc — do not hand-edit)

**Interfaces:**
- Produces: `gen.Queries` methods `CreateColumn`, `GetColumnByID`, `RenameColumn`, `DeleteColumn`, `ListColumnsByBoard`, `ReorderColumns`, `CreateCard`, `GetCardByID`, `UpdateCard`, `DeleteCard`, `ListCardsByColumn`, `SetCardColumn`, `ReorderCards`, and structs `gen.Column`, `gen.Card` — consumed by Task 6's `PostgresRepository`.
- `assignee_id` (nullable `uuid`) must generate as `*uuid.UUID` in `gen.Card` — this needs a new sqlc.yaml override (parallel to the existing nullable-`timestamptz` override), since the current plain `uuid` override applies to non-nullable columns and would otherwise win for `assignee_id` too, producing a bare `uuid.UUID` instead of a pointer.

- [ ] **Step 1: Add the nullable-uuid override to sqlc.yaml**

In `backend/sqlc.yaml`, add a new entry to the existing `overrides` list (keep the three existing entries — plain `uuid`, plain `timestamptz`, nullable `timestamptz` — exactly as they are):
```yaml
        overrides:
          - db_type: "uuid"
            go_type: "github.com/google/uuid.UUID"
          - db_type: "timestamptz"
            go_type: "time.Time"
          - db_type: "timestamptz"
            nullable: true
            go_type:
              import: "time"
              type: "Time"
              pointer: true
          - db_type: "uuid"
            nullable: true
            go_type:
              import: "github.com/google/uuid"
              type: "UUID"
              pointer: true
```

- [ ] **Step 2: Write the column queries**

`backend/db/queries/columns.sql`:
```sql
-- name: CreateColumn :one
INSERT INTO columns (id, board_id, title, position)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM columns WHERE board_id = $2))
RETURNING *;

-- name: GetColumnByID :one
SELECT * FROM columns WHERE id = $1;

-- name: RenameColumn :one
UPDATE columns SET title = $2, updated_at = now() WHERE id = $1
RETURNING *;

-- name: DeleteColumn :exec
DELETE FROM columns WHERE id = $1;

-- name: ListColumnsByBoard :many
SELECT * FROM columns WHERE board_id = $1 ORDER BY position ASC;

-- name: ReorderColumns :exec
UPDATE columns AS c
SET position = data.position, updated_at = now()
FROM (SELECT * FROM unnest(sqlc.arg(column_ids)::uuid[], sqlc.arg(positions)::int[])) AS data(id, position)
WHERE c.id = data.id AND c.board_id = sqlc.arg(board_id);
```

- [ ] **Step 3: Write the card queries**

`backend/db/queries/cards.sql`:
```sql
-- name: CreateCard :one
INSERT INTO cards (id, column_id, title, description, position, assignee_id, due_date)
VALUES ($1, $2, $3, $4, (SELECT COALESCE(MAX(position) + 1, 0) FROM cards WHERE column_id = $2), $5, $6)
RETURNING *;

-- name: GetCardByID :one
SELECT * FROM cards WHERE id = $1;

-- name: UpdateCard :one
UPDATE cards
SET title = $2, description = $3, assignee_id = $4, due_date = $5, updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteCard :exec
DELETE FROM cards WHERE id = $1;

-- name: ListCardsByColumn :many
SELECT * FROM cards WHERE column_id = $1 ORDER BY position ASC;

-- name: SetCardColumn :exec
UPDATE cards SET column_id = $2, updated_at = now() WHERE id = $1;

-- name: ReorderCards :exec
UPDATE cards AS c
SET position = data.position, updated_at = now()
FROM (SELECT * FROM unnest(sqlc.arg(card_ids)::uuid[], sqlc.arg(positions)::int[])) AS data(id, position)
WHERE c.id = data.id AND c.column_id = sqlc.arg(column_id);
```

- [ ] **Step 4: Generate the Go code**

Run (from `backend/`):
```bash
make sqlc-generate
```
Expected: `internal/platform/db/gen/` gains `columns.sql.go` and `cards.sql.go`, and `models.go` gains `Column` and `Card` structs. Confirm `Card.AssigneeID` is `*uuid.UUID` and `Card.DueDate` is `*time.Time` in the generated `models.go`.

**Note for Task 6:** sqlc's exact Go field-name casing for the `sqlc.arg(column_ids)`/`sqlc.arg(card_ids)` plural-array params (e.g. whether it generates `ColumnIds` or `ColumnIDs`) isn't 100% predictable without seeing the actual output. Task 6's repository code below uses `ColumnIds`/`CardIds` as a best guess — when implementing Task 6, check the real generated `ReorderColumnsParams`/`ReorderCardsParams` struct definitions in `boards.sql.go`/`cards.sql.go` first (`grep -n "type ReorderColumnsParams" -A5 backend/internal/platform/db/gen/boards.sql.go`) and adjust the field names used in `repository_postgres.go` to match exactly if they differ.

- [ ] **Step 5: Verify everything builds**

Run (from `backend/`):
```bash
go build ./...
```
Expected: no errors. Confirm `head -3 backend/go.mod` still shows `go 1.23` (no new dependency, no change expected).

- [ ] **Step 6: Commit**

```bash
git add backend/sqlc.yaml backend/db/queries backend/internal/platform/db/gen
git commit -m "feat(backend): generate type-safe column and card queries with sqlc"
```

---

### Task 3: `board.Service.EnsureMember` export + card domain/repository/fakes

**Files:**
- Modify: `backend/internal/board/service.go` (add one exported method)
- Create: `backend/internal/card/domain.go`
- Create: `backend/internal/card/repository.go`
- Create: `backend/internal/card/repository_fake_test.go`

**Interfaces:**
- Produces: `(*board.Service).EnsureMember(ctx, boardID, userID uuid.UUID) error` — the exported form of `board`'s existing private `requireMember`, letting other packages enforce board membership without re-implementing the `GetBoardMember` lookup.
- Produces: `card.Column`, `card.Card`, domain errors (`ErrColumnNotFound`, `ErrCardNotFound`, `ErrAssigneeNotFound`), `card.ErrNotFound` (repository-layer sentinel), `card.Repository` interface, `card.BoardAuthorizer` interface, `fakeRepository`, `fakeBoardAuthorizer` — consumed by Tasks 4, 5, 6, 7.

- [ ] **Step 1: Export `EnsureMember` on the board service**

Append to `backend/internal/board/service.go` (no new imports needed — it uses only what's already imported):
```go
// EnsureMember reports whether userID is a member of boardID, returning
// ErrNotAMember if not. It's the exported form of requireMember, so other
// packages (e.g. card) can enforce board membership without
// reimplementing the GetBoardMember lookup themselves.
func (s *Service) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	_, err := s.requireMember(ctx, boardID, userID)
	return err
}
```

- [ ] **Step 2: Run the board package's tests to confirm nothing broke**

Run (from `backend/`): `go test ./internal/board/... -v`
Expected: PASS (no existing test exercises this new method yet — that's fine, it's a pure addition).

- [ ] **Step 3: Define card domain types and errors**

`backend/internal/card/domain.go`:
```go
package card

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Column struct {
	ID        uuid.UUID
	BoardID   uuid.UUID
	Title     string
	Position  int32
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Card struct {
	ID          uuid.UUID
	ColumnID    uuid.UUID
	Title       string
	Description string
	Position    int32
	AssigneeID  *uuid.UUID
	DueDate     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

var (
	ErrColumnNotFound   = errors.New("column not found")
	ErrCardNotFound     = errors.New("card not found")
	ErrAssigneeNotFound = errors.New("assignee is not a registered user")
)
```

- [ ] **Step 4: Define the repository and board-authorizer interfaces**

`backend/internal/card/repository.go`:
```go
package card

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateColumn(ctx context.Context, c Column) (Column, error)
	GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error)
	RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error)
	DeleteColumn(ctx context.Context, id uuid.UUID) error
	ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error)
	ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error

	CreateCard(ctx context.Context, c Card) (Card, error)
	GetCardByID(ctx context.Context, id uuid.UUID) (Card, error)
	UpdateCard(ctx context.Context, c Card) (Card, error)
	DeleteCard(ctx context.Context, id uuid.UUID) error
	ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error)
	SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error
	ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error
}

// BoardAuthorizer checks board membership. Implemented by *board.Service
// via its EnsureMember method — card's service depends on this interface,
// never on the board package directly.
type BoardAuthorizer interface {
	EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error
}
```

- [ ] **Step 5: Write the fake repository and fake authorizer**

`backend/internal/card/repository_fake_test.go`:
```go
package card

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	mu      sync.Mutex
	columns map[uuid.UUID]Column
	cards   map[uuid.UUID]Card
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		columns: make(map[uuid.UUID]Column),
		cards:   make(map[uuid.UUID]Card),
	}
}

func (f *fakeRepository) CreateColumn(ctx context.Context, c Column) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	c.Position = int32(f.countColumnsForBoard(c.BoardID))
	c.CreatedAt = now
	c.UpdatedAt = now
	f.columns[c.ID] = c
	return c, nil
}

func (f *fakeRepository) countColumnsForBoard(boardID uuid.UUID) int {
	n := 0
	for _, c := range f.columns {
		if c.BoardID == boardID {
			n++
		}
	}
	return n
}

func (f *fakeRepository) GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.columns[id]
	if !ok {
		return Column{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepository) RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.columns[id]
	if !ok {
		return Column{}, ErrNotFound
	}
	c.Title = title
	c.UpdatedAt = time.Now()
	f.columns[id] = c
	return c, nil
}

func (f *fakeRepository) DeleteColumn(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.columns, id)
	for cardID, card := range f.cards {
		if card.ColumnID == id {
			delete(f.cards, cardID)
		}
	}
	return nil
}

func (f *fakeRepository) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Column
	for _, c := range f.columns {
		if c.BoardID == boardID {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result, nil
}

func (f *fakeRepository) ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range orderedColumnIDs {
		c, ok := f.columns[id]
		if !ok || c.BoardID != boardID {
			continue
		}
		c.Position = int32(i)
		c.UpdatedAt = time.Now()
		f.columns[id] = c
	}
	return nil
}

func (f *fakeRepository) CreateCard(ctx context.Context, c Card) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	c.Position = int32(f.countCardsForColumn(c.ColumnID))
	c.CreatedAt = now
	c.UpdatedAt = now
	f.cards[c.ID] = c
	return c, nil
}

func (f *fakeRepository) countCardsForColumn(columnID uuid.UUID) int {
	n := 0
	for _, c := range f.cards {
		if c.ColumnID == columnID {
			n++
		}
	}
	return n
}

func (f *fakeRepository) GetCardByID(ctx context.Context, id uuid.UUID) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[id]
	if !ok {
		return Card{}, ErrNotFound
	}
	return c, nil
}

func (f *fakeRepository) UpdateCard(ctx context.Context, c Card) (Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.cards[c.ID]
	if !ok {
		return Card{}, ErrNotFound
	}
	existing.Title = c.Title
	existing.Description = c.Description
	existing.AssigneeID = c.AssigneeID
	existing.DueDate = c.DueDate
	existing.UpdatedAt = time.Now()
	f.cards[c.ID] = existing
	return existing, nil
}

func (f *fakeRepository) DeleteCard(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.cards, id)
	return nil
}

func (f *fakeRepository) ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Card
	for _, c := range f.cards {
		if c.ColumnID == columnID {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Position < result[j].Position })
	return result, nil
}

func (f *fakeRepository) SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.cards[cardID]
	if !ok {
		return ErrNotFound
	}
	c.ColumnID = columnID
	c.UpdatedAt = time.Now()
	f.cards[cardID] = c
	return nil
}

func (f *fakeRepository) ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range orderedCardIDs {
		c, ok := f.cards[id]
		if !ok || c.ColumnID != columnID {
			continue
		}
		c.Position = int32(i)
		c.UpdatedAt = time.Now()
		f.cards[id] = c
	}
	return nil
}

var errFakeNotAMember = errors.New("not a member")

type fakeBoardAuthorizer struct {
	membersByBoard map[uuid.UUID]map[uuid.UUID]bool
}

func newFakeBoardAuthorizer() *fakeBoardAuthorizer {
	return &fakeBoardAuthorizer{membersByBoard: make(map[uuid.UUID]map[uuid.UUID]bool)}
}

func (f *fakeBoardAuthorizer) addMember(boardID, userID uuid.UUID) {
	if f.membersByBoard[boardID] == nil {
		f.membersByBoard[boardID] = make(map[uuid.UUID]bool)
	}
	f.membersByBoard[boardID][userID] = true
}

func (f *fakeBoardAuthorizer) EnsureMember(ctx context.Context, boardID, userID uuid.UUID) error {
	if f.membersByBoard[boardID] != nil && f.membersByBoard[boardID][userID] {
		return nil
	}
	return errFakeNotAMember
}

func TestFakeRepository_CreateColumnAssignsSequentialPositions(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()
	boardID := uuid.New()

	first, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	require.Equal(t, int32(0), first.Position)

	second, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)
	require.Equal(t, int32(1), second.Position)
}
```

- [ ] **Step 6: Run the tests and confirm they pass**

Run (from `backend/`): `go build ./... && go test ./internal/card/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/board/service.go backend/internal/card
git commit -m "feat(backend): export board.Service.EnsureMember and add card domain, repository, and test fakes"
```

---

### Task 4: Card-package service — column operations + `ListBoardColumns`

**Files:**
- Create: `backend/internal/card/service.go`
- Create: `backend/internal/card/service_test.go`

**Interfaces:**
- Consumes: `card.Repository`, `card.BoardAuthorizer` (Task 3), `fakeRepository`/`fakeBoardAuthorizer` (Task 3, test-only).
- Produces: `card.Service`, `card.NewService(repo Repository, board BoardAuthorizer) *Service`, `(*Service).CreateColumn(ctx, boardID, requesterID uuid.UUID, title string) (Column, error)`, `(*Service).RenameColumn(ctx, columnID, requesterID uuid.UUID, title string) (Column, error)`, `(*Service).DeleteColumn(ctx, columnID, requesterID uuid.UUID) error`, `(*Service).ReorderColumns(ctx, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error`, `(*Service).ListBoardColumns(ctx, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error)`, `ColumnWithCards{Column, Cards []Card}` — consumed by Task 5, 7, 8.

- [ ] **Step 1: Write the failing tests**

`backend/internal/card/service_test.go`:
```go
package card

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestService_CreateColumn_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	stranger := uuid.New()
	boardAuth.addMember(boardID, member)

	_, err := svc.CreateColumn(ctx, boardID, stranger, "To Do")
	require.Error(t, err)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	require.Equal(t, "To Do", column.Title)
	require.Equal(t, boardID, column.BoardID)
}

func TestService_RenameColumn_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	renamed, err := svc.RenameColumn(ctx, column.ID, member, "Backlog")
	require.NoError(t, err)
	require.Equal(t, "Backlog", renamed.Title)

	_, err = svc.RenameColumn(ctx, column.ID, uuid.New(), "Hacked")
	require.Error(t, err)
}

func TestService_DeleteColumn_UnknownColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()

	err := svc.DeleteColumn(ctx, uuid.New(), uuid.New())
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_ReorderColumns_PersistsOrder(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	first, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	second, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)

	err = svc.ReorderColumns(ctx, boardID, member, []uuid.UUID{second.ID, first.ID})
	require.NoError(t, err)

	columns, err := svc.ListBoardColumns(ctx, boardID, member)
	require.NoError(t, err)
	require.Len(t, columns, 2)
	require.Equal(t, second.ID, columns[0].ID)
	require.Equal(t, first.ID, columns[1].ID)
}

func TestService_ListBoardColumns_IncludesCards(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	_, err = repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: column.ID, Title: "Write plan"})
	require.NoError(t, err)

	columns, err := svc.ListBoardColumns(ctx, boardID, member)
	require.NoError(t, err)
	require.Len(t, columns, 1)
	require.Len(t, columns[0].Cards, 1)
	require.Equal(t, "Write plan", columns[0].Cards[0].Title)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/card/... -run TestService -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 3: Implement the service with column operations and `ListBoardColumns`**

`backend/internal/card/service.go`:
```go
package card

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	board BoardAuthorizer
}

func NewService(repo Repository, board BoardAuthorizer) *Service {
	return &Service{repo: repo, board: board}
}

func (s *Service) CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error) {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return Column{}, err
	}
	return s.repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: title})
}

func (s *Service) RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Column{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Column{}, err
	}
	return s.repo.RenameColumn(ctx, columnID, title)
}

func (s *Service) DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return err
	}
	return s.repo.DeleteColumn(ctx, columnID)
}

func (s *Service) ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	if err := s.board.EnsureMember(ctx, boardID, requesterID); err != nil {
		return err
	}
	return s.repo.ReorderColumns(ctx, boardID, orderedColumnIDs)
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
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/card/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/card/service.go backend/internal/card/service_test.go
git commit -m "feat(backend): implement card-package column service and ListBoardColumns"
```

---

### Task 5: Card-package service — card operations + `reorderWithInsert`

**Files:**
- Modify: `backend/internal/card/service.go` (add card operations and the reorder helper)
- Modify: `backend/internal/card/service_test.go` (add tests)

**Interfaces:**
- Produces: `(*Service).CreateCard(ctx, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)`, `(*Service).UpdateCard(ctx, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)`, `(*Service).DeleteCard(ctx, cardID, requesterID uuid.UUID) error`, `(*Service).MoveCard(ctx, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error)`, `reorderWithInsert(cards []Card, movingCardID uuid.UUID, targetPosition int) []uuid.UUID` — consumed by Task 7, 8.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/card/service_test.go` (add `"time"` to the import block):
```go
func TestService_CreateCard_Success(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)

	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "details", nil, nil)
	require.NoError(t, err)
	require.Equal(t, "Write plan", card.Title)
	require.Equal(t, column.ID, card.ColumnID)
}

func TestService_CreateCard_UnknownColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()

	_, err := svc.CreateCard(ctx, uuid.New(), uuid.New(), "Write plan", "", nil, nil)
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestService_UpdateCard_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	due := time.Now().Add(24 * time.Hour)
	updated, err := svc.UpdateCard(ctx, card.ID, member, "Write plan v2", "updated details", nil, &due)
	require.NoError(t, err)
	require.Equal(t, "Write plan v2", updated.Title)
	require.Equal(t, "updated details", updated.Description)

	_, err = svc.UpdateCard(ctx, card.ID, uuid.New(), "Hacked", "", nil, nil)
	require.Error(t, err)
}

func TestService_DeleteCard_Success(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, column.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	err = svc.DeleteCard(ctx, card.ID, member)
	require.NoError(t, err)

	_, err = repo.GetCardByID(ctx, card.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_MoveCard_WithinSameColumn(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	column, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	first, err := svc.CreateCard(ctx, column.ID, member, "First", "", nil, nil)
	require.NoError(t, err)
	second, err := svc.CreateCard(ctx, column.ID, member, "Second", "", nil, nil)
	require.NoError(t, err)

	_, err = svc.MoveCard(ctx, first.ID, member, column.ID, 1)
	require.NoError(t, err)

	cards, err := repo.ListCardsByColumn(ctx, column.ID)
	require.NoError(t, err)
	require.Len(t, cards, 2)
	require.Equal(t, second.ID, cards[0].ID)
	require.Equal(t, first.ID, cards[1].ID)
}

func TestService_MoveCard_AcrossColumns(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardID := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardID, member)

	todo, err := svc.CreateColumn(ctx, boardID, member, "To Do")
	require.NoError(t, err)
	doing, err := svc.CreateColumn(ctx, boardID, member, "Doing")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, todo.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	moved, err := svc.MoveCard(ctx, card.ID, member, doing.ID, 0)
	require.NoError(t, err)
	require.Equal(t, doing.ID, moved.ColumnID)

	todoCards, err := repo.ListCardsByColumn(ctx, todo.ID)
	require.NoError(t, err)
	require.Empty(t, todoCards)

	doingCards, err := repo.ListCardsByColumn(ctx, doing.ID)
	require.NoError(t, err)
	require.Len(t, doingCards, 1)
	require.Equal(t, card.ID, doingCards[0].ID)
}

func TestService_MoveCard_RejectsCrossBoardMove(t *testing.T) {
	repo := newFakeRepository()
	boardAuth := newFakeBoardAuthorizer()
	svc := NewService(repo, boardAuth)
	ctx := context.Background()
	boardA := uuid.New()
	boardB := uuid.New()
	member := uuid.New()
	boardAuth.addMember(boardA, member)
	boardAuth.addMember(boardB, member)

	columnA, err := svc.CreateColumn(ctx, boardA, member, "To Do")
	require.NoError(t, err)
	columnB, err := svc.CreateColumn(ctx, boardB, member, "To Do")
	require.NoError(t, err)
	card, err := svc.CreateCard(ctx, columnA.ID, member, "Write plan", "", nil, nil)
	require.NoError(t, err)

	_, err = svc.MoveCard(ctx, card.ID, member, columnB.ID, 0)
	require.True(t, errors.Is(err, ErrColumnNotFound))
}

func TestReorderWithInsert(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	cards := []Card{{ID: a}, {ID: b}, {ID: c}}

	result := reorderWithInsert(cards, b, 0)

	require.Equal(t, []uuid.UUID{b, a, c}, result)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/card/... -run "TestService_CreateCard|TestService_UpdateCard|TestService_DeleteCard|TestService_MoveCard|TestReorderWithInsert" -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the card operations and the reorder helper**

Append to `backend/internal/card/service.go` (add `"time"` to the import block):
```go
func (s *Service) CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	column, err := s.repo.GetColumnByID(ctx, columnID)
	if err != nil {
		return Card{}, mapColumnErr(err)
	}
	if err := s.board.EnsureMember(ctx, column.BoardID, requesterID); err != nil {
		return Card{}, err
	}
	return s.repo.CreateCard(ctx, Card{
		ID:          uuid.New(),
		ColumnID:    columnID,
		Title:       title,
		Description: description,
		AssigneeID:  assigneeID,
		DueDate:     dueDate,
	})
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
	return s.repo.UpdateCard(ctx, existing)
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
	return s.repo.DeleteCard(ctx, cardID)
}

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

	return s.repo.GetCardByID(ctx, cardID)
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

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/card/... -v`
Expected: PASS (all `card` package tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/card/service.go backend/internal/card/service_test.go
git commit -m "feat(backend): implement card-package card service and reorderWithInsert"
```

---

### Task 6: Postgres repository (integration-tested)

**Files:**
- Create: `backend/internal/card/repository_postgres.go`
- Create: `backend/internal/card/repository_postgres_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `gen.Queries` (Task 2), `card.Repository` (Task 3), `dbtest.NewPool` (Phase 1, Task 12).
- Produces: `card.NewPostgresRepository(q *gen.Queries) *PostgresRepository` implementing `card.Repository` — consumed by Task 8.

- [ ] **Step 1: Write the failing integration tests**

`backend/internal/card/repository_postgres_test.go`:
```go
//go:build integration

package card

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func createTestBoard(t *testing.T, ctx context.Context, q *gen.Queries, ownerEmail string) (boardID, ownerID uuid.UUID) {
	t.Helper()
	owner, err := q.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: ownerEmail, PasswordHash: "hashed"})
	require.NoError(t, err)
	board, err := q.CreateBoardWithOwner(ctx, gen.CreateBoardWithOwnerParams{ID: uuid.New(), Name: "Test Board", OwnerID: owner.ID})
	require.NoError(t, err)
	return board.ID, owner.ID
}

func TestPostgresRepository_CreateAndReorderColumns(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, _ := createTestBoard(t, ctx, q, "owner1@example.com")

	first, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	require.Equal(t, int32(0), first.Position)

	second, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)
	require.Equal(t, int32(1), second.Position)

	require.NoError(t, repo.ReorderColumns(ctx, boardID, []uuid.UUID{second.ID, first.ID}))

	columns, err := repo.ListColumnsByBoard(ctx, boardID)
	require.NoError(t, err)
	require.Len(t, columns, 2)
	require.Equal(t, second.ID, columns[0].ID)
	require.Equal(t, first.ID, columns[1].ID)

	_, err = repo.GetColumnByID(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_CardLifecycleAndMove(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, ownerID := createTestBoard(t, ctx, q, "owner2@example.com")
	todo, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)
	doing, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "Doing"})
	require.NoError(t, err)

	card, err := repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: todo.ID, Title: "Write plan", AssigneeID: &ownerID})
	require.NoError(t, err)
	require.NotNil(t, card.AssigneeID)
	require.Equal(t, ownerID, *card.AssigneeID)

	require.NoError(t, repo.SetCardColumn(ctx, card.ID, doing.ID))
	require.NoError(t, repo.ReorderCards(ctx, doing.ID, []uuid.UUID{card.ID}))

	moved, err := repo.GetCardByID(ctx, card.ID)
	require.NoError(t, err)
	require.Equal(t, doing.ID, moved.ColumnID)

	require.NoError(t, repo.DeleteCard(ctx, card.ID))
	_, err = repo.GetCardByID(ctx, card.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_CreateCard_UnknownAssignee(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	boardID, _ := createTestBoard(t, ctx, q, "owner3@example.com")
	column, err := repo.CreateColumn(ctx, Column{ID: uuid.New(), BoardID: boardID, Title: "To Do"})
	require.NoError(t, err)

	unknownUser := uuid.New()
	_, err = repo.CreateCard(ctx, Card{ID: uuid.New(), ColumnID: column.ID, Title: "Write plan", AssigneeID: &unknownUser})
	require.ErrorIs(t, err, ErrAssigneeNotFound)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `make test-integration`
Expected: FAIL — `NewPostgresRepository` undefined. (Requires Docker running locally. Note: this test file's `createTestBoard` helper calls `q.CreateBoardWithOwner`, the atomic board-creation query Phase 2's final review added — confirm it's still present in `gen.Queries` before proceeding; it should be, since Phase 2 is already merged.)

- [ ] **Step 3: Implement the Postgres repository**

`backend/internal/card/repository_postgres.go`:
```go
package card

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateColumn(ctx context.Context, c Column) (Column, error) {
	row, err := r.q.CreateColumn(ctx, gen.CreateColumnParams{ID: c.ID, BoardID: c.BoardID, Title: c.Title})
	if err != nil {
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) GetColumnByID(ctx context.Context, id uuid.UUID) (Column, error) {
	row, err := r.q.GetColumnByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Column{}, ErrNotFound
		}
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) RenameColumn(ctx context.Context, id uuid.UUID, title string) (Column, error) {
	row, err := r.q.RenameColumn(ctx, gen.RenameColumnParams{ID: id, Title: title})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Column{}, ErrNotFound
		}
		return Column{}, err
	}
	return toDomainColumn(row), nil
}

func (r *PostgresRepository) DeleteColumn(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteColumn(ctx, id)
}

func (r *PostgresRepository) ListColumnsByBoard(ctx context.Context, boardID uuid.UUID) ([]Column, error) {
	rows, err := r.q.ListColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	columns := make([]Column, 0, len(rows))
	for _, row := range rows {
		columns = append(columns, toDomainColumn(row))
	}
	return columns, nil
}

func (r *PostgresRepository) ReorderColumns(ctx context.Context, boardID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	positions := make([]int32, len(orderedColumnIDs))
	for i := range orderedColumnIDs {
		positions[i] = int32(i)
	}
	return r.q.ReorderColumns(ctx, gen.ReorderColumnsParams{
		ColumnIds: orderedColumnIDs,
		Positions: positions,
		BoardID:   boardID,
	})
}

func (r *PostgresRepository) CreateCard(ctx context.Context, c Card) (Card, error) {
	row, err := r.q.CreateCard(ctx, gen.CreateCardParams{
		ID:          c.ID,
		ColumnID:    c.ColumnID,
		Title:       c.Title,
		Description: c.Description,
		AssigneeID:  c.AssigneeID,
		DueDate:     c.DueDate,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return Card{}, ErrAssigneeNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) GetCardByID(ctx context.Context, id uuid.UUID) (Card, error) {
	row, err := r.q.GetCardByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) UpdateCard(ctx context.Context, c Card) (Card, error) {
	row, err := r.q.UpdateCard(ctx, gen.UpdateCardParams{
		ID:          c.ID,
		Title:       c.Title,
		Description: c.Description,
		AssigneeID:  c.AssigneeID,
		DueDate:     c.DueDate,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return Card{}, ErrAssigneeNotFound
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return Card{}, ErrNotFound
		}
		return Card{}, err
	}
	return toDomainCard(row), nil
}

func (r *PostgresRepository) DeleteCard(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteCard(ctx, id)
}

func (r *PostgresRepository) ListCardsByColumn(ctx context.Context, columnID uuid.UUID) ([]Card, error) {
	rows, err := r.q.ListCardsByColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, row := range rows {
		cards = append(cards, toDomainCard(row))
	}
	return cards, nil
}

func (r *PostgresRepository) SetCardColumn(ctx context.Context, cardID, columnID uuid.UUID) error {
	return r.q.SetCardColumn(ctx, gen.SetCardColumnParams{ID: cardID, ColumnID: columnID})
}

func (r *PostgresRepository) ReorderCards(ctx context.Context, columnID uuid.UUID, orderedCardIDs []uuid.UUID) error {
	positions := make([]int32, len(orderedCardIDs))
	for i := range orderedCardIDs {
		positions[i] = int32(i)
	}
	return r.q.ReorderCards(ctx, gen.ReorderCardsParams{
		CardIds:   orderedCardIDs,
		Positions: positions,
		ColumnID:  columnID,
	})
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func toDomainColumn(row gen.Column) Column {
	return Column{
		ID:        row.ID,
		BoardID:   row.BoardID,
		Title:     row.Title,
		Position:  row.Position,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainCard(row gen.Card) Card {
	return Card{
		ID:          row.ID,
		ColumnID:    row.ColumnID,
		Title:       row.Title,
		Description: row.Description,
		Position:    row.Position,
		AssigneeID:  row.AssigneeID,
		DueDate:     row.DueDate,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `make test-integration`
Expected: PASS (requires Docker running locally).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/card/repository_postgres.go backend/internal/card/repository_postgres_test.go
git commit -m "feat(backend): implement Postgres-backed card repository with integration tests"
```

---

### Task 7: HTTP handlers + routes

**Files:**
- Create: `backend/internal/card/handler.go`
- Create: `backend/internal/card/handler_test.go`

**Interfaces:**
- Consumes: `Column`, `Card`, `ColumnWithCards`, domain errors (Tasks 3-5), `board.ErrNotAMember`, `board.ErrForbidden` (Phase 2, for error-code translation only), `middleware.UserIDFromContext`, `middleware.Auth` (Phase 1).
- Produces: `card.NewHandler(service cardService) *Handler`, `(*Handler).RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler)` mounting `/boards/{boardID}/columns...` and `/cards...` behind auth — consumed by Task 8.

- [ ] **Step 1: Write the failing tests**

`backend/internal/card/handler_test.go`:
```go
package card

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeCardService struct {
	createColumnFn    func(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error)
	renameColumnFn    func(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error)
	deleteColumnFn    func(ctx context.Context, columnID, requesterID uuid.UUID) error
	reorderColumnsFn  func(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error
	listBoardColumnsFn func(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error)
	createCardFn      func(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	updateCardFn      func(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	deleteCardFn      func(ctx context.Context, cardID, requesterID uuid.UUID) error
	moveCardFn        func(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error)
}

func (f *fakeCardService) CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error) {
	return f.createColumnFn(ctx, boardID, requesterID, title)
}
func (f *fakeCardService) RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error) {
	return f.renameColumnFn(ctx, columnID, requesterID, title)
}
func (f *fakeCardService) DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error {
	return f.deleteColumnFn(ctx, columnID, requesterID)
}
func (f *fakeCardService) ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error {
	return f.reorderColumnsFn(ctx, boardID, requesterID, orderedColumnIDs)
}
func (f *fakeCardService) ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error) {
	return f.listBoardColumnsFn(ctx, boardID, requesterID)
}
func (f *fakeCardService) CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	return f.createCardFn(ctx, columnID, requesterID, title, description, assigneeID, dueDate)
}
func (f *fakeCardService) UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
	return f.updateCardFn(ctx, cardID, requesterID, title, description, assigneeID, dueDate)
}
func (f *fakeCardService) DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error {
	return f.deleteCardFn(ctx, cardID, requesterID)
}
func (f *fakeCardService) MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error) {
	return f.moveCardFn(ctx, cardID, requesterID, targetColumnID, targetPosition)
}

func setupTestRouter(svc cardService) (chi.Router, func(userID uuid.UUID) string) {
	issuer := jwt.NewIssuer("test-secret", time.Hour)
	h := NewHandler(svc)
	r := chi.NewRouter()
	h.RegisterRoutes(r, middleware.Auth(issuer))
	tokenFor := func(userID uuid.UUID) string {
		token, err := issuer.IssueAccessToken(userID)
		if err != nil {
			panic(err)
		}
		return token
	}
	return r, tokenFor
}

func TestHandler_CreateColumn_Success(t *testing.T) {
	boardID := uuid.New()
	requester := uuid.New()
	svc := &fakeCardService{
		createColumnFn: func(ctx context.Context, bID, reqID uuid.UUID, title string) (Column, error) {
			require.Equal(t, boardID, bID)
			return Column{ID: uuid.New(), BoardID: bID, Title: title, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createColumnRequest{Title: "To Do"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+boardID.String()+"/columns/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp columnView
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "To Do", resp.Title)
}

func TestHandler_CreateColumn_RequiresAuth(t *testing.T) {
	svc := &fakeCardService{}
	r, _ := setupTestRouter(svc)

	body, _ := json.Marshal(createColumnRequest{Title: "To Do"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+uuid.New().String()+"/columns/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_CreateCard_Forbidden(t *testing.T) {
	requester := uuid.New()
	svc := &fakeCardService{
		createCardFn: func(ctx context.Context, columnID, reqID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error) {
			return Card{}, board.ErrNotAMember
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createCardRequest{Title: "Write plan"})
	req := httptest.NewRequest(http.MethodPost, "/cards/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_MoveCard_InvalidCardID(t *testing.T) {
	requester := uuid.New()
	svc := &fakeCardService{}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(moveCardRequest{ColumnID: uuid.New(), Position: 0})
	req := httptest.NewRequest(http.MethodPatch, "/cards/not-a-uuid/move", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/card/... -run TestHandler -v`
Expected: FAIL — `NewHandler` undefined.

- [ ] **Step 3: Implement the handlers**

`backend/internal/card/handler.go`:
```go
package card

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type cardService interface {
	CreateColumn(ctx context.Context, boardID, requesterID uuid.UUID, title string) (Column, error)
	RenameColumn(ctx context.Context, columnID, requesterID uuid.UUID, title string) (Column, error)
	DeleteColumn(ctx context.Context, columnID, requesterID uuid.UUID) error
	ReorderColumns(ctx context.Context, boardID, requesterID uuid.UUID, orderedColumnIDs []uuid.UUID) error
	ListBoardColumns(ctx context.Context, boardID, requesterID uuid.UUID) ([]ColumnWithCards, error)
	CreateCard(ctx context.Context, columnID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	UpdateCard(ctx context.Context, cardID, requesterID uuid.UUID, title, description string, assigneeID *uuid.UUID, dueDate *time.Time) (Card, error)
	DeleteCard(ctx context.Context, cardID, requesterID uuid.UUID) error
	MoveCard(ctx context.Context, cardID, requesterID, targetColumnID uuid.UUID, targetPosition int) (Card, error)
}

type Handler struct {
	service cardService
}

func NewHandler(service cardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards/{boardID}/columns", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListBoardColumns)
		r.Post("/", h.CreateColumn)
		r.Patch("/reorder", h.ReorderColumns)
		r.Patch("/{columnID}", h.RenameColumn)
		r.Delete("/{columnID}", h.DeleteColumn)
	})
	r.Route("/cards", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Post("/", h.CreateCard)
		r.Route("/{cardID}", func(r chi.Router) {
			r.Patch("/", h.UpdateCard)
			r.Delete("/", h.DeleteCard)
			r.Patch("/move", h.MoveCard)
		})
	})
}

type columnView struct {
	ID        string     `json:"id"`
	BoardID   string     `json:"board_id"`
	Title     string     `json:"title"`
	Position  int32      `json:"position"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Cards     []cardView `json:"cards,omitempty"`
}

type cardView struct {
	ID          string     `json:"id"`
	ColumnID    string     `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Position    int32      `json:"position"`
	AssigneeID  *string    `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type createColumnRequest struct {
	Title string `json:"title"`
}

type renameColumnRequest struct {
	Title string `json:"title"`
}

type reorderColumnsRequest struct {
	ColumnIDs []uuid.UUID `json:"column_ids"`
}

type createCardRequest struct {
	ColumnID    uuid.UUID  `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
}

type updateCardRequest struct {
	Title       string     `json:"title"`
	Description string     `json:"description"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	DueDate     *time.Time `json:"due_date"`
}

type moveCardRequest struct {
	ColumnID uuid.UUID `json:"column_id"`
	Position int       `json:"position"`
}

func (h *Handler) CreateColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	var req createColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	column, err := h.service.CreateColumn(r.Context(), boardID, userID, req.Title)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toColumnView(column, nil))
}

func (h *Handler) RenameColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	columnID, err := parseUUIDParam(r, "columnID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid column id")
		return
	}

	var req renameColumnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	column, err := h.service.RenameColumn(r.Context(), columnID, userID, req.Title)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toColumnView(column, nil))
}

func (h *Handler) DeleteColumn(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	columnID, err := parseUUIDParam(r, "columnID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid column id")
		return
	}

	if err := h.service.DeleteColumn(r.Context(), columnID, userID); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderColumns(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	var req reorderColumnsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if err := h.service.ReorderColumns(r.Context(), boardID, userID, req.ColumnIDs); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListBoardColumns(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boardID, err := parseUUIDParam(r, "boardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid board id")
		return
	}

	columns, err := h.service.ListBoardColumns(r.Context(), boardID, userID)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	views := make([]columnView, 0, len(columns))
	for _, c := range columns {
		views = append(views, toColumnView(c.Column, c.Cards))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) CreateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req createCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	card, err := h.service.CreateCard(r.Context(), req.ColumnID, userID, req.Title, req.Description, req.AssigneeID, req.DueDate)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toCardView(card))
}

func (h *Handler) UpdateCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cardID, err := parseUUIDParam(r, "cardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid card id")
		return
	}

	var req updateCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is required")
		return
	}

	card, err := h.service.UpdateCard(r.Context(), cardID, userID, req.Title, req.Description, req.AssigneeID, req.DueDate)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCardView(card))
}

func (h *Handler) DeleteCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cardID, err := parseUUIDParam(r, "cardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid card id")
		return
	}

	if err := h.service.DeleteCard(r.Context(), cardID, userID); err != nil {
		h.writeCardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) MoveCard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	cardID, err := parseUUIDParam(r, "cardID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid card id")
		return
	}

	var req moveCardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	card, err := h.service.MoveCard(r.Context(), cardID, userID, req.ColumnID, req.Position)
	if err != nil {
		h.writeCardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toCardView(card))
}

func (h *Handler) writeCardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, board.ErrNotAMember), errors.Is(err, board.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrColumnNotFound), errors.Is(err, ErrCardNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrAssigneeNotFound):
		writeError(w, http.StatusBadRequest, "assignee_not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toColumnView(c Column, cards []Card) columnView {
	view := columnView{
		ID:        c.ID.String(),
		BoardID:   c.BoardID.String(),
		Title:     c.Title,
		Position:  c.Position,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if cards != nil {
		views := make([]cardView, 0, len(cards))
		for _, card := range cards {
			views = append(views, toCardView(card))
		}
		view.Cards = views
	}
	return view
}

func toCardView(c Card) cardView {
	var assigneeID *string
	if c.AssigneeID != nil {
		s := c.AssigneeID.String()
		assigneeID = &s
	}
	return cardView{
		ID:          c.ID.String(),
		ColumnID:    c.ColumnID.String(),
		Title:       c.Title,
		Description: c.Description,
		Position:    c.Position,
		AssigneeID:  assigneeID,
		DueDate:     c.DueDate,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/card/... -v`
Expected: PASS (all `card` package unit tests, including the new handler tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/card/handler.go backend/internal/card/handler_test.go
git commit -m "feat(backend): add card-package HTTP handlers and routes"
```

---

### Task 8: Wire `main.go` + end-to-end integration test + README update

**Files:**
- Modify: `backend/cmd/api/main.go`
- Create: `backend/internal/card/e2e_test.go` (build tag `integration`, package `card_test`)
- Modify: `backend/README.md` (document the new endpoints)

**Interfaces:**
- Consumes: everything produced by Tasks 2, 4, 5, 6, 7, plus Phase 2's `board.NewService`/`board.NewHandler`.
- Produces: a fully wired running server with `/auth`, `/boards`, and now `/boards/{boardID}/columns` + `/cards` all mounted — this is what Phase 4 (realtime) will build on.

- [ ] **Step 1: Write the failing end-to-end test**

`backend/internal/card/e2e_test.go`:
```go
//go:build integration

package card_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/board"
	"github.com/MatheusCavalari/kanvas/backend/internal/card"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestCardFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)

	boardRepo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	boardService := board.NewService(boardRepo, userLookup)
	boardHandler := board.NewHandler(boardService)

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService)
	cardHandler := card.NewHandler(cardService)

	router := httpserver.NewRouter()
	authMiddleware := middleware.Auth(issuer)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)

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

	createColumnBody, _ := json.Marshal(map[string]string{"title": "To Do"})
	createColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(createColumnBody))
	createColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createColumnResp, err := client.Do(createColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createColumnResp.StatusCode)
	var columnCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createColumnResp.Body).Decode(&columnCreated))
	_ = createColumnResp.Body.Close()

	secondColumnBody, _ := json.Marshal(map[string]string{"title": "Doing"})
	secondColumnReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+boardCreated.ID+"/columns/", bytes.NewReader(secondColumnBody))
	secondColumnReq.Header.Set("Authorization", "Bearer "+ownerToken)
	secondColumnResp, err := client.Do(secondColumnReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, secondColumnResp.StatusCode)
	var secondColumnCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(secondColumnResp.Body).Decode(&secondColumnCreated))
	_ = secondColumnResp.Body.Close()

	createCardBody, _ := json.Marshal(map[string]interface{}{
		"column_id": columnCreated.ID,
		"title":     "Write the plan",
	})
	createCardReq, _ := http.NewRequest(http.MethodPost, server.URL+"/cards/", bytes.NewReader(createCardBody))
	createCardReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createCardResp, err := client.Do(createCardReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createCardResp.StatusCode)
	var cardCreated struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createCardResp.Body).Decode(&cardCreated))
	_ = createCardResp.Body.Close()

	moveBody, _ := json.Marshal(map[string]interface{}{
		"column_id": secondColumnCreated.ID,
		"position":  0,
	})
	moveReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/cards/"+cardCreated.ID+"/move", bytes.NewReader(moveBody))
	moveReq.Header.Set("Authorization", "Bearer "+ownerToken)
	moveResp, err := client.Do(moveReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, moveResp.StatusCode)
	_ = moveResp.Body.Close()

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+boardCreated.ID+"/columns/", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	var columns []struct {
		ID    string `json:"id"`
		Cards []struct {
			ID string `json:"id"`
		} `json:"cards"`
	}
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&columns))
	_ = listResp.Body.Close()
	require.Len(t, columns, 2)
	require.Empty(t, columns[0].Cards)
	require.Len(t, columns[1].Cards, 1)
	require.Equal(t, cardCreated.ID, columns[1].Cards[0].ID)

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/cards/"+cardCreated.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `backend/`): `make test-integration`
Expected: doesn't depend on `main.go`, so it should mostly compile and exercise real code from Tasks 2-7; if anything fails, read the failure carefully — it should point at a real logic gap, not a missing symbol.

- [ ] **Step 3: Wire `main.go` to mount the card handler**

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

	cardRepo := card.NewPostgresRepository(queries)
	cardService := card.NewService(cardRepo, boardService)
	cardHandler := card.NewHandler(cardService)

	router := httpserver.NewRouter()
	authHandler.RegisterRoutes(router, authMiddleware)
	boardHandler.RegisterRoutes(router, authMiddleware)
	cardHandler.RegisterRoutes(router, authMiddleware)

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

(Only the `card` import, `cardRepo`/`cardService`/`cardHandler` variables, and the extra `cardHandler.RegisterRoutes(...)` call are new versus the current file. Note `card.NewService(cardRepo, boardService)` passes the concrete `*board.Service` as the `BoardAuthorizer` interface argument — this is the only place the two packages' concrete types actually meet.)

- [ ] **Step 4: Run the integration test and confirm it passes**

Run (from `backend/`): `make test-integration`
Expected: PASS

- [ ] **Step 5: Smoke-test the real server manually**

Run (from `backend/`, with `docker compose up -d postgres` running):
```bash
make run
```
In another terminal (chain the register → create board → create column → create card flow, substituting the IDs/tokens you actually get back):
```bash
curl -i -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Ada Lovelace","email":"ada@example.com","password":"supersecret"}'

curl -i -X POST http://localhost:8080/boards/ \
  -H "Content-Type: application/json" -H "Authorization: Bearer <access_token>" \
  -d '{"name":"Sprint Board"}'

curl -i -X POST http://localhost:8080/boards/<board_id>/columns/ \
  -H "Content-Type: application/json" -H "Authorization: Bearer <access_token>" \
  -d '{"title":"To Do"}'

curl -i -X POST http://localhost:8080/cards/ \
  -H "Content-Type: application/json" -H "Authorization: Bearer <access_token>" \
  -d '{"column_id":"<column_id>","title":"Write the plan"}'
```
Expected: each call returns `201` with the created resource's JSON. Stop the server with Ctrl+C.

- [ ] **Step 6: Update `backend/README.md`**

Add a new section to `backend/README.md` (after the "Boards & members" section Phase 2 added), and update "Project layout" to include `internal/card/`:
```markdown
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
```

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/api/main.go backend/internal/card/e2e_test.go backend/README.md
git commit -m "feat(backend): wire card routes into main.go and add card flow integration test"
```

---

## Definition of Done

- `make test` and `make test-integration` both pass locally.
- A fresh `docker compose down -v && docker compose up -d --build` followed by register → create board → create column → create card → move card → list columns via `curl` all work end to end.
- `make lint` is clean.
- GitHub Actions is green on the pushed branch (the existing `backend-ci.yml` workflow needs no changes — it already runs `go test ./...` and `-tags=integration` across every package, including the new `internal/card`).
