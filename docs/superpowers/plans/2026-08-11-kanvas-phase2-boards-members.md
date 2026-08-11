# Kanvas — Phase 2: Boards & Members — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working, tested REST API for creating boards, listing a user's boards, and managing board membership (invite-by-email, remove, role-gated permissions), built on top of Phase 1's auth foundation.

**Architecture:** A new `internal/board` package following the exact Clean/Hexagonal shape established in Phase 1's `internal/auth`: `domain.go` (entities/errors), `repository.go` (interface) + `repository_postgres.go` (sqlc-backed implementation), `service.go` (business logic behind interfaces), `handler.go` (chi HTTP handlers). Board membership invitations resolve an email to a user ID via a small `UserLookup` interface, implemented by reusing the `gen.Queries.GetUserByEmail` query Phase 1 already generated — no new coupling to the `auth` package.

**Tech Stack:** Same as Phase 1 — Go 1.23, chi, sqlc + pgx v5, golang-migrate, testcontainers-go for integration tests, testify.

## Global Constraints

- Go module path: `github.com/MatheusCavalari/kanvas/backend`. Go version: **1.23** — `go.mod`'s `go` directive must stay at `go 1.23` even if this machine's installed toolchain is newer. This bit Phase 1 repeatedly: after any `go get`, run `go mod edit -go=1.23` then `go mod tidy`, and check `head -3 backend/go.mod`. If a new dependency's own `go.mod` requires a newer Go version, pin that dependency to an older compatible version instead of accepting the bump (see Phase 1's `git log` for precedent: pgx pinned to v5.7.6, testify to v1.8.1).
- This phase adds **no new third-party dependencies** — everything it needs (chi, pgx, sqlc-generated code, testcontainers, testify, golang-migrate) is already in `go.mod` from Phase 1. If a task genuinely needs something new, stop and ask rather than improvising.
- sqlc regeneration MUST go through Docker, not `go run`: `docker run --rm -v "$(pwd):/src" -w //src sqlc/sqlc:1.27.0 generate` (already wired as `backend/Makefile`'s `sqlc-generate` target) — a bare `go run .../sqlc generate` panics with a WASM crash on this Windows machine.
- Error responses are a JSON envelope: `{"error": {"code": "...", "message": "..."}}`, written via a small per-package `writeError`/`writeJSON` helper pair (see `internal/auth/handler.go` and `internal/platform/middleware/auth.go` for the established pattern — it's deliberately duplicated per package rather than shared, to avoid a cross-package coupling for five lines of code).
- All board routes require authentication (unlike `/auth`, where only `/me` was protected) — the entire `/boards` subrouter is wrapped in the auth middleware from `internal/platform/middleware`.
- Integration tests are gated behind `//go:build integration` and use `internal/platform/db/dbtest.NewPool(t)`, which already applies every migration in `backend/db/migrations` (including this phase's new ones) via `runtime.Caller`-based path resolution — no changes needed to `dbtest.go` itself.
- `internal/platform/httpserver.NewRouter()` takes no arguments (deliberate Phase 1 design so `httpserver` never depends on a feature package). `main.go` calls `NewRouter()` once, then calls `RegisterRoutes(router, authMiddleware)` for each feature handler in turn.
- Migrations continue the existing numbering: `000003`, `000004` (Phase 1 used `000001`, `000002`).
- Tests: unit tests use in-memory fakes (no `integration` tag). Integration tests use real Postgres via testcontainers-go. Both must pass before a task is done.

---

## Task Overview

1. Migrations: `boards`, `board_members`
2. sqlc queries: boards, board_members + regenerate
3. Board domain types + repository interface + `UserLookup` interface + test fakes
4. Board service: `CreateBoard`, `ListBoards`, `GetBoard` + permission helpers
5. Board service: `RenameBoard`, `DeleteBoard`, `InviteMember`, `RemoveMember`, `ListMembers`
6. Postgres repository + `UserLookupAdapter` (integration-tested)
7. Board HTTP handlers + routes
8. Wire `main.go` + end-to-end integration test + README update

---

### Task 1: Migrations — `boards`, `board_members`

**Files:**
- Create: `backend/db/migrations/000003_create_boards_table.up.sql`
- Create: `backend/db/migrations/000003_create_boards_table.down.sql`
- Create: `backend/db/migrations/000004_create_board_members_table.up.sql`
- Create: `backend/db/migrations/000004_create_board_members_table.down.sql`

**Interfaces:**
- Produces: `boards` table (`id uuid pk`, `name`, `owner_id fk -> users(id)`, `created_at`, `updated_at`) and `board_members` table (`board_id fk -> boards(id)`, `user_id fk -> users(id)`, `role text check (owner|member)`, `created_at`, composite PK `(board_id, user_id)`) — consumed by Task 2's sqlc queries and Task 6's repository.

No Go code in this task — verified by running the migrations against the local Postgres.

- [ ] **Step 1: Create the `boards` table migration**

`backend/db/migrations/000003_create_boards_table.up.sql`:
```sql
CREATE TABLE boards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_boards_owner_id ON boards(owner_id);
```

`backend/db/migrations/000003_create_boards_table.down.sql`:
```sql
DROP TABLE IF EXISTS boards;
```

- [ ] **Step 2: Create the `board_members` table migration**

`backend/db/migrations/000004_create_board_members_table.up.sql`:
```sql
CREATE TABLE board_members (
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner', 'member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (board_id, user_id)
);

CREATE INDEX idx_board_members_user_id ON board_members(user_id);
```

`backend/db/migrations/000004_create_board_members_table.down.sql`:
```sql
DROP TABLE IF EXISTS board_members;
```

- [ ] **Step 3: Run the migrations against the local Postgres**

Ensure Postgres is running (`docker compose up -d postgres` from the repo root if it isn't already), then run (from `backend/`):
```bash
make migrate-up
```
Expected: output ending with something like `000004/u create_board_members_table (X.XXXXXXXms)`, no errors.

- [ ] **Step 4: Verify the tables exist**

Run (from repo root):
```bash
docker compose exec postgres psql -U kanvas -d kanvas -c "\dt"
```
Expected: lists `users`, `refresh_tokens`, `boards`, `board_members`, `schema_migrations`.

- [ ] **Step 5: Commit**

```bash
git add backend/db/migrations
git commit -m "feat(backend): add boards and board_members migrations"
```

---

### Task 2: sqlc queries — boards, board_members

**Files:**
- Create: `backend/db/queries/boards.sql`
- Create: `backend/db/queries/board_members.sql`
- Modify: `backend/internal/platform/db/gen/*.go` (regenerated by sqlc — do not hand-edit; a hand-written "sqlc output" shortcut caused a multi-round fix in Phase 1 Task 6, do not repeat that)

**Interfaces:**
- Produces: `gen.Queries` methods `CreateBoard`, `GetBoardByID`, `UpdateBoardName`, `DeleteBoard`, `ListBoardsForUser`, `AddBoardMember`, `RemoveBoardMember`, `GetBoardMember`, `ListBoardMembers`, and structs `gen.Board`, `gen.BoardMember` plus their params structs — consumed by Task 6's `PostgresRepository`.
- Note: sqlc only generates a `*Params` struct when a query has 2+ parameters. `GetBoardByID`, `DeleteBoard`, and `ListBoardsForUser` each take exactly one parameter, so they take that value directly (e.g. `GetBoardByID(ctx, id)`), matching the existing pattern from Phase 1's `GetUserByID`. `ListBoardMembers` also takes exactly one parameter (`board_id`) and is called the same way.

- [ ] **Step 1: Write the board queries**

`backend/db/queries/boards.sql`:
```sql
-- name: CreateBoard :one
INSERT INTO boards (id, name, owner_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetBoardByID :one
SELECT * FROM boards WHERE id = $1;

-- name: UpdateBoardName :one
UPDATE boards SET name = $2, updated_at = now() WHERE id = $1
RETURNING *;

-- name: DeleteBoard :exec
DELETE FROM boards WHERE id = $1;

-- name: ListBoardsForUser :many
SELECT b.* FROM boards b
JOIN board_members bm ON bm.board_id = b.id
WHERE bm.user_id = $1
ORDER BY b.created_at DESC;
```

- [ ] **Step 2: Write the board member queries**

`backend/db/queries/board_members.sql`:
```sql
-- name: AddBoardMember :exec
INSERT INTO board_members (board_id, user_id, role)
VALUES ($1, $2, $3);

-- name: RemoveBoardMember :exec
DELETE FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: GetBoardMember :one
SELECT * FROM board_members WHERE board_id = $1 AND user_id = $2;

-- name: ListBoardMembers :many
SELECT * FROM board_members WHERE board_id = $1
ORDER BY created_at ASC;
```

- [ ] **Step 3: Generate the Go code**

Run (from `backend/`):
```bash
make sqlc-generate
```
Expected: `internal/platform/db/gen/` now also contains `boards.sql.go` and `board_members.sql.go` (exact filenames may vary slightly by sqlc version), and `models.go` gains `Board` and `BoardMember` structs.

- [ ] **Step 4: Verify everything builds**

Run (from `backend/`):
```bash
go build ./...
```
Expected: no errors. Also confirm `head -3 backend/go.mod` still shows `go 1.23` (this step shouldn't touch go.mod at all, since no new dependency is added — if it somehow did change, investigate before proceeding).

- [ ] **Step 5: Commit**

```bash
git add backend/db/queries backend/internal/platform/db/gen
git commit -m "feat(backend): generate type-safe board queries with sqlc"
```

---

### Task 3: Board domain types, repository interface, `UserLookup` interface, test fakes

**Files:**
- Create: `backend/internal/board/domain.go`
- Create: `backend/internal/board/repository.go`
- Create: `backend/internal/board/repository_fake_test.go`
- Create: `backend/internal/board/user_lookup_fake_test.go`

**Interfaces:**
- Produces: `board.Board`, `board.Member`, `board.Role` (`RoleOwner`, `RoleMember`), domain errors (`ErrNotAMember`, `ErrForbidden`, `ErrAlreadyMember`, `ErrMemberUserNotFound`, `ErrCannotRemoveOwner`), `board.Repository` interface, `board.UserLookup` interface, `board.ErrNotFound` (repository-layer sentinel) — consumed by every later `internal/board` task.

- [ ] **Step 1: Define domain types and errors**

`backend/internal/board/domain.go`:
```go
package board

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleMember Role = "member"
)

type Board struct {
	ID        uuid.UUID
	Name      string
	OwnerID   uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Member struct {
	BoardID   uuid.UUID
	UserID    uuid.UUID
	Role      Role
	CreatedAt time.Time
}

var (
	ErrNotAMember         = errors.New("user is not a member of this board")
	ErrForbidden          = errors.New("only the board owner can perform this action")
	ErrAlreadyMember      = errors.New("user is already a member of this board")
	ErrMemberUserNotFound = errors.New("no user found with that email")
	ErrCannotRemoveOwner  = errors.New("cannot remove the board owner")
)
```

- [ ] **Step 2: Define the repository and user-lookup interfaces**

`backend/internal/board/repository.go`:
```go
package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

type Repository interface {
	CreateBoard(ctx context.Context, b Board) (Board, error)
	GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error)
	UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error)
	DeleteBoard(ctx context.Context, id uuid.UUID) error
	ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error)

	AddMember(ctx context.Context, m Member) error
	RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error
	GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error)
	ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error)
}

// UserLookup resolves an email to a user ID. It exists so the board
// package never depends on the auth package directly — main.go wires a
// concrete implementation (Task 6's UserLookupAdapter) that happens to
// reuse auth's generated GetUserByEmail query under the hood.
type UserLookup interface {
	UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
}
```

- [ ] **Step 3: Write the failing sanity test for the in-memory fake repository**

`backend/internal/board/repository_fake_test.go`:
```go
package board

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeRepository struct {
	mu      sync.Mutex
	boards  map[uuid.UUID]Board
	members map[string]Member
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		boards:  make(map[uuid.UUID]Board),
		members: make(map[string]Member),
	}
}

func memberKey(boardID, userID uuid.UUID) string {
	return boardID.String() + "|" + userID.String()
}

func (f *fakeRepository) CreateBoard(ctx context.Context, b Board) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	b.CreatedAt = now
	b.UpdatedAt = now
	f.boards[b.ID] = b
	return b, nil
}

func (f *fakeRepository) GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boards[id]
	if !ok {
		return Board{}, ErrNotFound
	}
	return b, nil
}

func (f *fakeRepository) UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.boards[id]
	if !ok {
		return Board{}, ErrNotFound
	}
	b.Name = name
	b.UpdatedAt = time.Now()
	f.boards[id] = b
	return b, nil
}

func (f *fakeRepository) DeleteBoard(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.boards[id]; !ok {
		return ErrNotFound
	}
	delete(f.boards, id)
	for k, m := range f.members {
		if m.BoardID == id {
			delete(f.members, k)
		}
	}
	return nil
}

func (f *fakeRepository) ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Board
	for _, m := range f.members {
		if m.UserID == userID {
			if b, ok := f.boards[m.BoardID]; ok {
				result = append(result, b)
			}
		}
	}
	return result, nil
}

func (f *fakeRepository) AddMember(ctx context.Context, m Member) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m.CreatedAt = time.Now()
	f.members[memberKey(m.BoardID, m.UserID)] = m
	return nil
}

func (f *fakeRepository) RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := memberKey(boardID, userID)
	if _, ok := f.members[key]; !ok {
		return ErrNotFound
	}
	delete(f.members, key)
	return nil
}

func (f *fakeRepository) GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.members[memberKey(boardID, userID)]
	if !ok {
		return Member{}, ErrNotFound
	}
	return m, nil
}

func (f *fakeRepository) ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var result []Member
	for _, m := range f.members {
		if m.BoardID == boardID {
			result = append(result, m)
		}
	}
	return result, nil
}

func TestFakeRepository_CreateBoardAddsNoMembers(t *testing.T) {
	repo := newFakeRepository()
	ctx := context.Background()

	owner := uuid.New()
	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Sprint Board", OwnerID: owner})
	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)

	_, err = repo.GetMember(ctx, board.ID, owner)
	require.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 4: Run the test and confirm it passes**

Run (from `backend/`): `go test ./internal/board/... -v`
Expected: PASS. (This test doubles as documentation that `CreateBoard` alone does NOT add the owner as a member — that's Task 4's `Service.CreateBoard`'s job, keeping the repository a thin data-access layer.)

- [ ] **Step 5: Add the fake user lookup**

`backend/internal/board/user_lookup_fake_test.go`:
```go
package board

import (
	"context"

	"github.com/google/uuid"
)

type fakeUserLookup struct {
	usersByEmail map[string]uuid.UUID
}

func newFakeUserLookup() *fakeUserLookup {
	return &fakeUserLookup{usersByEmail: make(map[string]uuid.UUID)}
}

func (f *fakeUserLookup) UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	id, ok := f.usersByEmail[email]
	if !ok {
		return uuid.UUID{}, ErrMemberUserNotFound
	}
	return id, nil
}
```

- [ ] **Step 6: Verify the module builds**

Run (from `backend/`): `go build ./... && go test ./internal/board/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/board
git commit -m "feat(backend): add board domain types, repository interface, and test fakes"
```

---

### Task 4: Board service — `CreateBoard`, `ListBoards`, `GetBoard`

**Files:**
- Create: `backend/internal/board/service.go`
- Create: `backend/internal/board/service_test.go`

**Interfaces:**
- Consumes: `board.Repository`, `board.UserLookup` (Task 3), `fakeRepository`/`fakeUserLookup` (Task 3, test-only).
- Produces: `board.Service`, `board.NewService(repo Repository, users UserLookup) *Service`, `(*Service).CreateBoard(ctx, ownerID uuid.UUID, name string) (Board, error)`, `(*Service).ListBoards(ctx, userID uuid.UUID) ([]Board, error)`, `(*Service).GetBoard(ctx, boardID, requesterID uuid.UUID) (Board, error)`, and the private helpers `requireMember`/`requireOwner` — consumed by Task 5, 7, 8.

- [ ] **Step 1: Write the failing tests**

`backend/internal/board/service_test.go`:
```go
package board

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestService_CreateBoard_AddsOwnerAsMember(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Sprint Board")

	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)
	require.Equal(t, owner, board.OwnerID)

	member, err := repo.GetMember(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Equal(t, RoleOwner, member.Role)
}

func TestService_ListBoards_OnlyReturnsMemberBoards(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	other := uuid.New()

	_, err := svc.CreateBoard(ctx, owner, "Owner's Board")
	require.NoError(t, err)

	boards, err := svc.ListBoards(ctx, other)
	require.NoError(t, err)
	require.Empty(t, boards)

	boards, err = svc.ListBoards(ctx, owner)
	require.NoError(t, err)
	require.Len(t, boards, 1)
}

func TestService_GetBoard_NonMemberForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	stranger := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Sprint Board")
	require.NoError(t, err)

	_, err = svc.GetBoard(ctx, board.ID, stranger)
	require.True(t, errors.Is(err, ErrNotAMember))

	fetched, err := svc.GetBoard(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Equal(t, board.ID, fetched.ID)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/board/... -run TestService -v`
Expected: FAIL — `NewService` undefined.

- [ ] **Step 3: Implement the service with `CreateBoard`, `ListBoards`, `GetBoard`**

`backend/internal/board/service.go`:
```go
package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type Service struct {
	repo  Repository
	users UserLookup
}

func NewService(repo Repository, users UserLookup) *Service {
	return &Service{repo: repo, users: users}
}

func (s *Service) CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
	board, err := s.repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: name, OwnerID: ownerID})
	if err != nil {
		return Board{}, err
	}
	if err := s.repo.AddMember(ctx, Member{BoardID: board.ID, UserID: ownerID, Role: RoleOwner}); err != nil {
		return Board{}, err
	}
	return board, nil
}

func (s *Service) ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	return s.repo.ListBoardsForUser(ctx, userID)
}

func (s *Service) GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error) {
	if _, err := s.requireMember(ctx, boardID, requesterID); err != nil {
		return Board{}, err
	}
	return s.repo.GetBoardByID(ctx, boardID)
}

// requireMember returns ErrNotAMember both when the board doesn't exist
// and when it exists but the user isn't on it — deliberately not
// distinguishing the two, so a non-member can't probe for board
// existence via the error code.
func (s *Service) requireMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	m, err := s.repo.GetMember(ctx, boardID, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Member{}, ErrNotAMember
		}
		return Member{}, err
	}
	return m, nil
}

func (s *Service) requireOwner(ctx context.Context, boardID, userID uuid.UUID) error {
	m, err := s.requireMember(ctx, boardID, userID)
	if err != nil {
		return err
	}
	if m.Role != RoleOwner {
		return ErrForbidden
	}
	return nil
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/board/... -run TestService -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/board/service.go backend/internal/board/service_test.go
git commit -m "feat(backend): implement board service CreateBoard, ListBoards, GetBoard"
```

---

### Task 5: Board service — `RenameBoard`, `DeleteBoard`, `InviteMember`, `RemoveMember`, `ListMembers`

**Files:**
- Modify: `backend/internal/board/service.go` (add the five methods)
- Modify: `backend/internal/board/service_test.go` (add tests)

**Interfaces:**
- Produces: `(*Service).RenameBoard(ctx, boardID, requesterID uuid.UUID, name string) (Board, error)`, `(*Service).DeleteBoard(ctx, boardID, requesterID uuid.UUID) error`, `(*Service).InviteMember(ctx, boardID, requesterID uuid.UUID, email string) (Member, error)`, `(*Service).RemoveMember(ctx, boardID, requesterID, targetUserID uuid.UUID) error`, `(*Service).ListMembers(ctx, boardID, requesterID uuid.UUID) ([]Member, error)` — consumed by Task 7, 8.

- [ ] **Step 1: Write the failing tests**

Append to `backend/internal/board/service_test.go`:
```go
func TestService_RenameBoard_RequiresMembership(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Old Name")
	require.NoError(t, err)

	renamed, err := svc.RenameBoard(ctx, board.ID, owner, "New Name")
	require.NoError(t, err)
	require.Equal(t, "New Name", renamed.Name)

	_, err = svc.RenameBoard(ctx, board.ID, uuid.New(), "Hacked Name")
	require.True(t, errors.Is(err, ErrNotAMember))
}

func TestService_DeleteBoard_OnlyOwner(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "To Delete")
	require.NoError(t, err)

	err = svc.DeleteBoard(ctx, board.ID, uuid.New())
	require.True(t, errors.Is(err, ErrNotAMember))

	err = svc.DeleteBoard(ctx, board.ID, owner)
	require.NoError(t, err)

	_, err = repo.GetBoardByID(ctx, board.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestService_InviteMember_Success(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	invitee := uuid.New()
	users.usersByEmail["friend@example.com"] = invitee

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	member, err := svc.InviteMember(ctx, board.ID, owner, "Friend@Example.com")
	require.NoError(t, err)
	require.Equal(t, invitee, member.UserID)
	require.Equal(t, RoleMember, member.Role)
}

func TestService_InviteMember_NotOwnerForbidden(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	member := uuid.New()
	users.usersByEmail["member@example.com"] = member
	users.usersByEmail["another@example.com"] = uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "member@example.com")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, member, "another@example.com")
	require.True(t, errors.Is(err, ErrForbidden))
}

func TestService_InviteMember_AlreadyMember(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	users.usersByEmail["owner@example.com"] = owner

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, owner, "owner@example.com")
	require.True(t, errors.Is(err, ErrAlreadyMember))
}

func TestService_InviteMember_UnknownEmail(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	_, err = svc.InviteMember(ctx, board.ID, owner, "nobody@example.com")
	require.True(t, errors.Is(err, ErrMemberUserNotFound))
}

func TestService_RemoveMember_CannotRemoveSelf(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)

	err = svc.RemoveMember(ctx, board.ID, owner, owner)
	require.True(t, errors.Is(err, ErrCannotRemoveOwner))
}

func TestService_RemoveMember_Success(t *testing.T) {
	repo := newFakeRepository()
	users := newFakeUserLookup()
	svc := NewService(repo, users)
	ctx := context.Background()
	owner := uuid.New()
	invitee := uuid.New()
	users.usersByEmail["friend@example.com"] = invitee

	board, err := svc.CreateBoard(ctx, owner, "Team Board")
	require.NoError(t, err)
	_, err = svc.InviteMember(ctx, board.ID, owner, "friend@example.com")
	require.NoError(t, err)

	err = svc.RemoveMember(ctx, board.ID, owner, invitee)
	require.NoError(t, err)

	members, err := svc.ListMembers(ctx, board.ID, owner)
	require.NoError(t, err)
	require.Len(t, members, 1)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/board/... -run "TestService_RenameBoard|TestService_DeleteBoard|TestService_InviteMember|TestService_RemoveMember" -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement `RenameBoard`, `DeleteBoard`, `InviteMember`, `RemoveMember`, `ListMembers`**

Append to `backend/internal/board/service.go` (and add `"strings"` to the existing import block):
```go
func (s *Service) RenameBoard(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error) {
	if _, err := s.requireMember(ctx, boardID, requesterID); err != nil {
		return Board{}, err
	}
	return s.repo.UpdateBoardName(ctx, boardID, name)
}

func (s *Service) DeleteBoard(ctx context.Context, boardID, requesterID uuid.UUID) error {
	if err := s.requireOwner(ctx, boardID, requesterID); err != nil {
		return err
	}
	return s.repo.DeleteBoard(ctx, boardID)
}

func (s *Service) InviteMember(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error) {
	if err := s.requireOwner(ctx, boardID, requesterID); err != nil {
		return Member{}, err
	}

	userID, err := s.users.UserIDByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return Member{}, err
	}

	if _, err := s.repo.GetMember(ctx, boardID, userID); err == nil {
		return Member{}, ErrAlreadyMember
	} else if !errors.Is(err, ErrNotFound) {
		return Member{}, err
	}

	member := Member{BoardID: boardID, UserID: userID, Role: RoleMember}
	if err := s.repo.AddMember(ctx, member); err != nil {
		return Member{}, err
	}
	return member, nil
}

func (s *Service) RemoveMember(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error {
	if err := s.requireOwner(ctx, boardID, requesterID); err != nil {
		return err
	}
	if targetUserID == requesterID {
		return ErrCannotRemoveOwner
	}
	return s.repo.RemoveMember(ctx, boardID, targetUserID)
}

func (s *Service) ListMembers(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error) {
	if _, err := s.requireMember(ctx, boardID, requesterID); err != nil {
		return nil, err
	}
	return s.repo.ListMembers(ctx, boardID)
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `go test ./internal/board/... -v`
Expected: PASS (all `Service` tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/board/service.go backend/internal/board/service_test.go
git commit -m "feat(backend): implement board service RenameBoard, DeleteBoard, InviteMember, RemoveMember, ListMembers"
```

---

### Task 6: Postgres repository + `UserLookupAdapter` (integration-tested)

**Files:**
- Create: `backend/internal/board/repository_postgres.go`
- Create: `backend/internal/board/repository_postgres_test.go` (build tag `integration`)

**Interfaces:**
- Consumes: `gen.Queries` (Task 2), `board.Repository`, `board.UserLookup` (Task 3), `dbtest.NewPool` (already exists from Phase 1 Task 12).
- Produces: `board.NewPostgresRepository(q *gen.Queries) *PostgresRepository` implementing `board.Repository`; `board.NewUserLookupAdapter(q *gen.Queries) *UserLookupAdapter` implementing `board.UserLookup` — consumed by Task 8.

- [ ] **Step 1: Write the failing integration tests**

`backend/internal/board/repository_postgres_test.go`:
```go
//go:build integration

package board

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

func createTestUser(t *testing.T, ctx context.Context, q *gen.Queries, email string) uuid.UUID {
	t.Helper()
	user, err := q.CreateUser(ctx, gen.CreateUserParams{
		ID:           uuid.New(),
		Name:         "Test User",
		Email:        email,
		PasswordHash: "hashed",
	})
	require.NoError(t, err)
	return user.ID
}

func TestPostgresRepository_CreateAndGetBoard(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "owner@example.com")

	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Sprint Board", OwnerID: owner})
	require.NoError(t, err)
	require.Equal(t, "Sprint Board", board.Name)

	fetched, err := repo.GetBoardByID(ctx, board.ID)
	require.NoError(t, err)
	require.Equal(t, board.ID, fetched.ID)

	_, err = repo.GetBoardByID(ctx, uuid.New())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresRepository_MemberLifecycle(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	repo := NewPostgresRepository(q)
	ctx := context.Background()

	owner := createTestUser(t, ctx, q, "owner2@example.com")
	member := createTestUser(t, ctx, q, "member2@example.com")

	board, err := repo.CreateBoard(ctx, Board{ID: uuid.New(), Name: "Team Board", OwnerID: owner})
	require.NoError(t, err)

	require.NoError(t, repo.AddMember(ctx, Member{BoardID: board.ID, UserID: owner, Role: RoleOwner}))
	require.NoError(t, repo.AddMember(ctx, Member{BoardID: board.ID, UserID: member, Role: RoleMember}))

	members, err := repo.ListMembers(ctx, board.ID)
	require.NoError(t, err)
	require.Len(t, members, 2)

	require.NoError(t, repo.RemoveMember(ctx, board.ID, member))

	_, err = repo.GetMember(ctx, board.ID, member)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestUserLookupAdapter_UserIDByEmail(t *testing.T) {
	pool := dbtest.NewPool(t)
	q := gen.New(pool)
	adapter := NewUserLookupAdapter(q)
	ctx := context.Background()

	userID := createTestUser(t, ctx, q, "lookup@example.com")

	found, err := adapter.UserIDByEmail(ctx, "lookup@example.com")
	require.NoError(t, err)
	require.Equal(t, userID, found)

	_, err = adapter.UserIDByEmail(ctx, "missing@example.com")
	require.ErrorIs(t, err, ErrMemberUserNotFound)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `make test-integration`
Expected: FAIL — `NewPostgresRepository` undefined. (Requires Docker running locally.)

- [ ] **Step 3: Implement the Postgres repository and user-lookup adapter**

`backend/internal/board/repository_postgres.go`:
```go
package board

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
)

type PostgresRepository struct {
	q *gen.Queries
}

func NewPostgresRepository(q *gen.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) CreateBoard(ctx context.Context, b Board) (Board, error) {
	row, err := r.q.CreateBoard(ctx, gen.CreateBoardParams{ID: b.ID, Name: b.Name, OwnerID: b.OwnerID})
	if err != nil {
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) GetBoardByID(ctx context.Context, id uuid.UUID) (Board, error) {
	row, err := r.q.GetBoardByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Board{}, ErrNotFound
		}
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) UpdateBoardName(ctx context.Context, id uuid.UUID, name string) (Board, error) {
	row, err := r.q.UpdateBoardName(ctx, gen.UpdateBoardNameParams{ID: id, Name: name})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Board{}, ErrNotFound
		}
		return Board{}, err
	}
	return toDomainBoard(row), nil
}

func (r *PostgresRepository) DeleteBoard(ctx context.Context, id uuid.UUID) error {
	return r.q.DeleteBoard(ctx, id)
}

func (r *PostgresRepository) ListBoardsForUser(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	rows, err := r.q.ListBoardsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	boards := make([]Board, 0, len(rows))
	for _, row := range rows {
		boards = append(boards, toDomainBoard(row))
	}
	return boards, nil
}

func (r *PostgresRepository) AddMember(ctx context.Context, m Member) error {
	return r.q.AddBoardMember(ctx, gen.AddBoardMemberParams{BoardID: m.BoardID, UserID: m.UserID, Role: string(m.Role)})
}

func (r *PostgresRepository) RemoveMember(ctx context.Context, boardID, userID uuid.UUID) error {
	return r.q.RemoveBoardMember(ctx, gen.RemoveBoardMemberParams{BoardID: boardID, UserID: userID})
}

func (r *PostgresRepository) GetMember(ctx context.Context, boardID, userID uuid.UUID) (Member, error) {
	row, err := r.q.GetBoardMember(ctx, gen.GetBoardMemberParams{BoardID: boardID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, ErrNotFound
		}
		return Member{}, err
	}
	return toDomainMember(row), nil
}

func (r *PostgresRepository) ListMembers(ctx context.Context, boardID uuid.UUID) ([]Member, error) {
	rows, err := r.q.ListBoardMembers(ctx, boardID)
	if err != nil {
		return nil, err
	}
	members := make([]Member, 0, len(rows))
	for _, row := range rows {
		members = append(members, toDomainMember(row))
	}
	return members, nil
}

func toDomainBoard(row gen.Board) Board {
	return Board{
		ID:        row.ID,
		Name:      row.Name,
		OwnerID:   row.OwnerID,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func toDomainMember(row gen.BoardMember) Member {
	return Member{
		BoardID:   row.BoardID,
		UserID:    row.UserID,
		Role:      Role(row.Role),
		CreatedAt: row.CreatedAt,
	}
}

// UserLookupAdapter implements board.UserLookup by reusing the
// GetUserByEmail query sqlc already generated for the auth package
// (Phase 1, Task 6). This lets board resolve invite emails to user IDs
// without importing the auth package at all.
type UserLookupAdapter struct {
	q *gen.Queries
}

func NewUserLookupAdapter(q *gen.Queries) *UserLookupAdapter {
	return &UserLookupAdapter{q: q}
}

func (a *UserLookupAdapter) UserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	row, err := a.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, ErrMemberUserNotFound
		}
		return uuid.UUID{}, err
	}
	return row.ID, nil
}
```

- [ ] **Step 4: Run the tests and confirm they pass**

Run (from `backend/`): `make test-integration`
Expected: PASS (requires Docker running locally).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/board/repository_postgres.go backend/internal/board/repository_postgres_test.go
git commit -m "feat(backend): implement Postgres-backed board repository with integration tests"
```

---

### Task 7: Board HTTP handlers + routes

**Files:**
- Create: `backend/internal/board/handler.go`
- Create: `backend/internal/board/handler_test.go`

**Interfaces:**
- Consumes: `Board`, `Member`, domain errors (Task 3-5), `middleware.UserIDFromContext`, `middleware.Auth` (already exist from Phase 1 Task 13).
- Produces: `board.NewHandler(service boardService) *Handler`, `(*Handler).RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler)` mounting the entire `/boards` subtree behind auth — consumed by Task 8.

- [ ] **Step 1: Write the failing tests**

`backend/internal/board/handler_test.go`:
```go
package board

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

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type fakeBoardService struct {
	createBoardFn  func(ctx context.Context, ownerID uuid.UUID, name string) (Board, error)
	listBoardsFn   func(ctx context.Context, userID uuid.UUID) ([]Board, error)
	getBoardFn     func(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error)
	renameBoardFn  func(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error)
	deleteBoardFn  func(ctx context.Context, boardID, requesterID uuid.UUID) error
	inviteMemberFn func(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error)
	removeMemberFn func(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error
	listMembersFn  func(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error)
}

func (f *fakeBoardService) CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
	return f.createBoardFn(ctx, ownerID, name)
}
func (f *fakeBoardService) ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error) {
	return f.listBoardsFn(ctx, userID)
}
func (f *fakeBoardService) GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error) {
	return f.getBoardFn(ctx, boardID, requesterID)
}
func (f *fakeBoardService) RenameBoard(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error) {
	return f.renameBoardFn(ctx, boardID, requesterID, name)
}
func (f *fakeBoardService) DeleteBoard(ctx context.Context, boardID, requesterID uuid.UUID) error {
	return f.deleteBoardFn(ctx, boardID, requesterID)
}
func (f *fakeBoardService) InviteMember(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error) {
	return f.inviteMemberFn(ctx, boardID, requesterID, email)
}
func (f *fakeBoardService) RemoveMember(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error {
	return f.removeMemberFn(ctx, boardID, requesterID, targetUserID)
}
func (f *fakeBoardService) ListMembers(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error) {
	return f.listMembersFn(ctx, boardID, requesterID)
}

// setupTestRouter wires a real JWT issuer and the real auth middleware
// around the board handler, so tests exercise genuine token-based auth
// (issue a real token, send it as a real Authorization header) instead
// of bypassing the middleware — there is no way to inject a fake user ID
// into the request context from outside the middleware package, since
// its context key is unexported by design.
func setupTestRouter(svc boardService) (chi.Router, func(userID uuid.UUID) string) {
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

func TestHandler_CreateBoard_Success(t *testing.T) {
	owner := uuid.New()
	boardID := uuid.New()
	svc := &fakeBoardService{
		createBoardFn: func(ctx context.Context, ownerID uuid.UUID, name string) (Board, error) {
			require.Equal(t, owner, ownerID)
			return Board{ID: boardID, Name: name, OwnerID: ownerID, CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(createBoardRequest{Name: "Sprint Board"})
	req := httptest.NewRequest(http.MethodPost, "/boards/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(owner))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	var resp boardView
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "Sprint Board", resp.Name)
}

func TestHandler_CreateBoard_RequiresAuth(t *testing.T) {
	svc := &fakeBoardService{}
	r, _ := setupTestRouter(svc)

	body, _ := json.Marshal(createBoardRequest{Name: "Sprint Board"})
	req := httptest.NewRequest(http.MethodPost, "/boards/", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandler_InviteMember_Forbidden(t *testing.T) {
	requester := uuid.New()
	boardID := uuid.New()
	svc := &fakeBoardService{
		inviteMemberFn: func(ctx context.Context, bID, reqID uuid.UUID, email string) (Member, error) {
			return Member{}, ErrForbidden
		},
	}
	r, tokenFor := setupTestRouter(svc)

	body, _ := json.Marshal(inviteMemberRequest{Email: "friend@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/boards/"+boardID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandler_DeleteBoard_InvalidBoardID(t *testing.T) {
	requester := uuid.New()
	svc := &fakeBoardService{}
	r, tokenFor := setupTestRouter(svc)

	req := httptest.NewRequest(http.MethodDelete, "/boards/not-a-uuid", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFor(requester))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 2: Run the tests and confirm they fail**

Run (from `backend/`): `go test ./internal/board/... -run TestHandler -v`
Expected: FAIL — `NewHandler` undefined.

- [ ] **Step 3: Implement the handlers**

`backend/internal/board/handler.go`:
```go
package board

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

type boardService interface {
	CreateBoard(ctx context.Context, ownerID uuid.UUID, name string) (Board, error)
	ListBoards(ctx context.Context, userID uuid.UUID) ([]Board, error)
	GetBoard(ctx context.Context, boardID, requesterID uuid.UUID) (Board, error)
	RenameBoard(ctx context.Context, boardID, requesterID uuid.UUID, name string) (Board, error)
	DeleteBoard(ctx context.Context, boardID, requesterID uuid.UUID) error
	InviteMember(ctx context.Context, boardID, requesterID uuid.UUID, email string) (Member, error)
	RemoveMember(ctx context.Context, boardID, requesterID, targetUserID uuid.UUID) error
	ListMembers(ctx context.Context, boardID, requesterID uuid.UUID) ([]Member, error)
}

type Handler struct {
	service boardService
}

func NewHandler(service boardService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/boards", func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", h.ListBoards)
		r.Post("/", h.CreateBoard)
		r.Route("/{boardID}", func(r chi.Router) {
			r.Get("/", h.GetBoard)
			r.Patch("/", h.RenameBoard)
			r.Delete("/", h.DeleteBoard)
			r.Get("/members", h.ListMembers)
			r.Post("/members", h.InviteMember)
			r.Delete("/members/{userID}", h.RemoveMember)
		})
	})
}

type boardView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type memberView struct {
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type createBoardRequest struct {
	Name string `json:"name"`
}

type renameBoardRequest struct {
	Name string `json:"name"`
}

type inviteMemberRequest struct {
	Email string `json:"email"`
}

func (h *Handler) CreateBoard(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req createBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	board, err := h.service.CreateBoard(r.Context(), userID, req.Name)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toBoardView(board))
}

func (h *Handler) ListBoards(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	boards, err := h.service.ListBoards(r.Context(), userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	views := make([]boardView, 0, len(boards))
	for _, b := range boards {
		views = append(views, toBoardView(b))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) GetBoard(w http.ResponseWriter, r *http.Request) {
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

	board, err := h.service.GetBoard(r.Context(), boardID, userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toBoardView(board))
}

func (h *Handler) RenameBoard(w http.ResponseWriter, r *http.Request) {
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

	var req renameBoardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "name is required")
		return
	}

	board, err := h.service.RenameBoard(r.Context(), boardID, userID, req.Name)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toBoardView(board))
}

func (h *Handler) DeleteBoard(w http.ResponseWriter, r *http.Request) {
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

	if err := h.service.DeleteBoard(r.Context(), boardID, userID); err != nil {
		h.writeBoardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
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

	var req inviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}

	member, err := h.service.InviteMember(r.Context(), boardID, userID, req.Email)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toMemberView(member))
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
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

	targetUserID, err := parseUUIDParam(r, "userID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid user id")
		return
	}

	if err := h.service.RemoveMember(r.Context(), boardID, userID, targetUserID); err != nil {
		h.writeBoardError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
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

	members, err := h.service.ListMembers(r.Context(), boardID, userID)
	if err != nil {
		h.writeBoardError(w, err)
		return
	}

	views := make([]memberView, 0, len(members))
	for _, m := range members {
		views = append(views, toMemberView(m))
	}
	writeJSON(w, http.StatusOK, views)
}

func (h *Handler) writeBoardError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotAMember):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, ErrAlreadyMember):
		writeError(w, http.StatusConflict, "already_member", err.Error())
	case errors.Is(err, ErrMemberUserNotFound):
		writeError(w, http.StatusNotFound, "user_not_found", err.Error())
	case errors.Is(err, ErrCannotRemoveOwner):
		writeError(w, http.StatusBadRequest, "cannot_remove_owner", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func parseUUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, name))
}

func toBoardView(b Board) boardView {
	return boardView{
		ID:        b.ID.String(),
		Name:      b.Name,
		OwnerID:   b.OwnerID.String(),
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

func toMemberView(m Member) memberView {
	return memberView{
		UserID:    m.UserID.String(),
		Role:      string(m.Role),
		CreatedAt: m.CreatedAt,
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

Run (from `backend/`): `go test ./internal/board/... -v`
Expected: PASS (all `board` package unit tests, including the new handler tests)

- [ ] **Step 5: Commit**

```bash
git add backend/internal/board/handler.go backend/internal/board/handler_test.go
git commit -m "feat(backend): add board HTTP handlers and routes"
```

---

### Task 8: Wire `main.go` + end-to-end integration test + README update

**Files:**
- Modify: `backend/cmd/api/main.go`
- Create: `backend/internal/board/e2e_test.go` (build tag `integration`, package `board_test`)
- Modify: `backend/README.md` (document the new endpoints)

**Interfaces:**
- Consumes: everything produced by Tasks 2, 4, 5, 6, 7.
- Produces: a fully wired running server with both `/auth` and `/boards` mounted — this is what Phase 3 (columns & cards) will build on.

- [ ] **Step 1: Write the failing end-to-end test**

`backend/internal/board/e2e_test.go`:
```go
//go:build integration

package board_test

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
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/dbtest"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/db/gen"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/httpserver"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/jwt"
	"github.com/MatheusCavalari/kanvas/backend/internal/platform/middleware"
)

func TestBoardFlow_EndToEnd(t *testing.T) {
	pool := dbtest.NewPool(t)
	queries := gen.New(pool)
	ctx := context.Background()

	owner, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Owner", Email: "owner@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)
	invitee, err := queries.CreateUser(ctx, gen.CreateUserParams{ID: uuid.New(), Name: "Invitee", Email: "invitee@example.com", PasswordHash: "hashed"})
	require.NoError(t, err)

	issuer := jwt.NewIssuer("test-secret", time.Hour)
	repo := board.NewPostgresRepository(queries)
	userLookup := board.NewUserLookupAdapter(queries)
	service := board.NewService(repo, userLookup)
	handler := board.NewHandler(service)

	router := httpserver.NewRouter()
	handler.RegisterRoutes(router, middleware.Auth(issuer))

	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()

	ownerToken, err := issuer.IssueAccessToken(owner.ID)
	require.NoError(t, err)

	createBody, _ := json.Marshal(map[string]string{"name": "Sprint Board"})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer "+ownerToken)
	createResp, err := client.Do(createReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&created))
	_ = createResp.Body.Close()

	inviteBody, _ := json.Marshal(map[string]string{"email": "invitee@example.com"})
	inviteReq, _ := http.NewRequest(http.MethodPost, server.URL+"/boards/"+created.ID+"/members", bytes.NewReader(inviteBody))
	inviteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	inviteResp, err := client.Do(inviteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, inviteResp.StatusCode)
	_ = inviteResp.Body.Close()

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/boards/"+created.ID+"/members", nil)
	listReq.Header.Set("Authorization", "Bearer "+ownerToken)
	listResp, err := client.Do(listReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var members []map[string]string
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&members))
	require.Len(t, members, 2)
	_ = listResp.Body.Close()

	removeReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID+"/members/"+invitee.ID.String(), nil)
	removeReq.Header.Set("Authorization", "Bearer "+ownerToken)
	removeResp, err := client.Do(removeReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, removeResp.StatusCode)
	_ = removeResp.Body.Close()

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/boards/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+ownerToken)
	deleteResp, err := client.Do(deleteReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleteResp.StatusCode)
	_ = deleteResp.Body.Close()
}
```

- [ ] **Step 2: Run the test and confirm it fails**

Run (from `backend/`): `make test-integration`
Expected: this test doesn't depend on `main.go`, so it should mostly compile and exercise real code from Tasks 2-7; if anything fails, read the failure output before proceeding — it should point at a real logic gap, not a missing symbol.

- [ ] **Step 3: Wire `main.go` to mount the board handler**

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

	router := httpserver.NewRouter()
	authHandler.RegisterRoutes(router, authMiddleware)
	boardHandler.RegisterRoutes(router, authMiddleware)

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

(Only the `board` import, `board*` variables, and the extra `boardHandler.RegisterRoutes(...)` call are new versus the current file — `queries := gen.New(pool)` replaces two separate `gen.New(pool)` calls with one shared instance, a small cleanup since it's now used by two feature packages.)

- [ ] **Step 4: Run the integration test and confirm it passes**

Run (from `backend/`): `make test-integration`
Expected: PASS

- [ ] **Step 5: Smoke-test the real server manually**

Run (from `backend/`, with `docker compose up -d postgres` running):
```bash
make run
```
In another terminal (replace the token and board id with what you actually get back):
```bash
curl -i -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"name":"Ada Lovelace","email":"ada@example.com","password":"supersecret"}'

curl -i -X POST http://localhost:8080/boards/ \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token from the register response>" \
  -d '{"name":"Sprint Board"}'
```
Expected: register returns `201` with an `access_token`; the board create call returns `201` with the new board's JSON. Stop the server with Ctrl+C.

- [ ] **Step 6: Update `backend/README.md`**

Add a new section to `backend/README.md` (after the existing "Try it" example), documenting the board endpoints:
```markdown
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
```

- [ ] **Step 7: Commit**

```bash
git add backend/cmd/api/main.go backend/internal/board/e2e_test.go backend/README.md
git commit -m "feat(backend): wire board routes into main.go and add board flow integration test"
```

---

## Definition of Done

- `make test` and `make test-integration` both pass locally.
- A fresh `docker compose down -v && docker compose up -d --build` followed by register → create board → invite → list members → remove member → delete board via `curl` all work end to end.
- `make lint` is clean.
- GitHub Actions is green on the pushed branch (the existing `backend-ci.yml` workflow needs no changes — it already runs `go test ./...` and `-tags=integration` across every package, including the new `internal/board`).
