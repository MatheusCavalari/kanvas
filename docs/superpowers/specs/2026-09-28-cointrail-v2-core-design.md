# CoinTrail v2 — Sub-Project 1: Core Financial Features

## Overview

Upgrade the existing CoinTrail personal finance REST API with four new financial domain features: Tags, Budgets, Goals, and Recurring Transactions. Each feature follows the project's established layered architecture (handlers → service → repository) and integrates with the existing transaction/category system.

**Goal:** Enrich CoinTrail with the core financial management features that differentiate a portfolio project — recurring income/expense automation, budget tracking, savings goals, and flexible transaction tagging.

**Existing Stack:** Clojure 1.12, Ring 1.12 + Compojure, next.jdbc + HoneySQL 2, buddy-auth (JWT), clojure.spec.alpha, PostgreSQL 16, Migratus, Docker Compose, GitHub Actions CI.

**Existing Schema:** `users`, `categories`, `transactions` (with indexes). Migrations numbered 001–006.

## Global Constraints

- Clojure 1.12 on JVM 21 (Eclipse Temurin)
- PostgreSQL 16 — all new tables use UUID PKs with `gen_random_uuid()`
- Migratus migrations — ONE SQL statement per `.up.sql` file, `IF NOT EXISTS` / `IF EXISTS` for idempotency
- All endpoints under `/api/` prefix, JWT Bearer auth on all except `/auth/*` and `/health`
- HoneySQL 2 for query building — `h/where` accepts multiple variadic clauses (valid)
- `BigDecimal` for all monetary calculations — use `.divide` with `RoundingMode/HALF_UP` scale 2
- Follow existing patterns: `parse-uuid` from `cointrail.util`, 400 on invalid UUIDs, `clojure.spec.alpha` for request validation
- One migration file per DDL statement (Migratus + pgjdbc limitation)
- Docker Compose: app (:8080) + postgres (:5432)
- Tests use `ring.mock` with real database (no mocks)
- Swagger spec at `/api/swagger.json` must be updated with all new endpoints

---

## Feature 1: Tags

Customizable labels for transactions. Many-to-many relationship.

### Data Model

**Table `tags` (migration 007):**
```sql
CREATE TABLE IF NOT EXISTS tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(30) NOT NULL,
    color VARCHAR(7),
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, name)
);
```

**Table `transaction_tags` (migration 008):**
```sql
CREATE TABLE IF NOT EXISTS transaction_tags (
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
```

### Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| GET | `/api/tags` | — | List user's tags |
| POST | `/api/tags` | `{name, color?}` | Create tag |
| PUT | `/api/tags/:id` | `{name?, color?}` | Update tag |
| DELETE | `/api/tags/:id` | — | Delete tag (cascades to associations) |
| PUT | `/api/transactions/:id/tags` | `{tag_ids: [uuid...]}` | Replace all tags on a transaction |
| GET | `/api/transactions/:id/tags` | — | List tags of a transaction |

### Integration with Existing Code

- `GET /api/transactions` gains optional query param `?tag=<uuid>` to filter by tag
- Transaction response objects include `tags: [{id, name, color}]` array
- `GET /api/transactions/:id` response includes `tags` array
- Swagger spec updated with tag endpoints and transaction response changes

### Validation Rules

- `name`: required, 1–30 chars
- `color`: optional, must match `^#[0-9a-fA-F]{6}$` if provided
- `tag_ids` in PUT: must all belong to the requesting user, max 10 per transaction
- Tag must belong to the requesting user for update/delete
- Empty body on PUT returns 400 "at least one field required"

### Files

- `src/cointrail/tags/handlers.clj`
- `src/cointrail/tags/service.clj`
- `src/cointrail/tags/repository.clj`
- `src/cointrail/tags/spec.clj`
- `test/cointrail/tags/handlers_test.clj`
- Modify: `src/cointrail/routes.clj`, `src/cointrail/transactions/repository.clj`, `src/cointrail/transactions/handlers.clj`, `src/cointrail/swagger.clj`

---

## Feature 2: Budgets

Monthly budgets per expense category with spend tracking.

### Data Model

**Table `budgets` (migration 009):**
```sql
CREATE TABLE IF NOT EXISTS budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(user_id, category_id)
);
```

No history table — actual spend is calculated dynamically by querying `transactions` for the target month.

### Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| GET | `/api/budgets` | — | List budgets with spend progress. Query: `?month=2026-09` (default: current month) |
| POST | `/api/budgets` | `{category_id, amount}` | Create budget |
| PUT | `/api/budgets/:id` | `{amount}` | Update limit |
| DELETE | `/api/budgets/:id` | — | Remove budget |

### Response Format

```json
{
  "id": "uuid",
  "category": {"id": "uuid", "name": "Alimentação", "icon": "🍔"},
  "amount": 800.00,
  "spent": 523.50,
  "remaining": 276.50,
  "percentage": 65.4,
  "status": "on_track"
}
```

### Business Rules

- **Status thresholds:** `on_track` (spent < 80% of amount), `warning` (80–100%), `over_budget` (> 100%)
- Category must be type `expense` and belong to the requesting user
- One budget per category (UNIQUE constraint)
- `percentage` uses `BigDecimal.divide` with `RoundingMode/HALF_UP` scale 1
- `spent` is `SUM(amount)` from `transactions` where `category_id` matches, `type = 'expense'`, and `date` falls within the target month
- `remaining` = `amount - spent` (can be negative when over budget)
- Month parameter parsed as `YearMonth`, defaults to `YearMonth/now`

### Files

- `src/cointrail/budgets/handlers.clj`
- `src/cointrail/budgets/service.clj`
- `src/cointrail/budgets/repository.clj`
- `src/cointrail/budgets/spec.clj`
- `test/cointrail/budgets/handlers_test.clj`
- Modify: `src/cointrail/routes.clj`, `src/cointrail/swagger.clj`

---

## Feature 3: Goals (Savings Goals)

Virtual piggy bank — users create goals with a target amount and optional deadline, then make manual deposits/withdrawals to track progress.

### Data Model

**Table `goals` (migration 010):**
```sql
CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    target_amount DECIMAL(12,2) NOT NULL CHECK (target_amount > 0),
    current_amount DECIMAL(12,2) NOT NULL DEFAULT 0 CHECK (current_amount >= 0),
    deadline DATE,
    status VARCHAR(12) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'reached', 'cancelled')),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

**Table `goal_deposits` (migration 011):**
```sql
CREATE TABLE IF NOT EXISTS goal_deposits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    amount DECIMAL(12,2) NOT NULL,
    note VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT now()
);
```

`current_amount` is denormalized on `goals` for query performance — updated in the service layer within a DB transaction on every deposit/withdraw.

### Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| GET | `/api/goals` | — | List goals with progress. Query: `?status=active\|reached\|cancelled` |
| POST | `/api/goals` | `{name, target_amount, deadline?}` | Create goal |
| PUT | `/api/goals/:id` | `{name?, target_amount?, deadline?}` | Update goal |
| DELETE | `/api/goals/:id` | — | Delete goal + deposits (cascade) |
| POST | `/api/goals/:id/deposit` | `{amount, note?}` | Deposit (amount > 0) |
| POST | `/api/goals/:id/withdraw` | `{amount, note?}` | Withdraw (amount > 0, cannot exceed current_amount) |
| GET | `/api/goals/:id/deposits` | — | List deposit/withdrawal history |

### Response Format

```json
{
  "id": "uuid",
  "name": "Viagem Europa",
  "target_amount": 15000.00,
  "current_amount": 4200.00,
  "percentage": 28.0,
  "deadline": "2027-06-01",
  "days_remaining": 246,
  "status": "active"
}
```

### Business Rules

- When `current_amount >= target_amount` after a deposit, status auto-changes to `reached`
- Withdraw cannot make `current_amount` negative — return 400 "insufficient balance"
- `days_remaining`: null if no deadline, 0 if deadline passed, otherwise `ChronoUnit/DAYS.between(today, deadline)`
- Deposits stored with positive `amount`, withdrawals with negative `amount` in `goal_deposits`
- Only `active` goals accept deposits/withdrawals — return 400 "goal is not active" otherwise
- `percentage` uses `BigDecimal.divide` with `RoundingMode/HALF_UP` scale 1
- Empty body on PUT returns 400 "at least one field required"
- Goals are independent from transactions — they do NOT interact with the recurring transaction scheduler

### Files

- `src/cointrail/goals/handlers.clj`
- `src/cointrail/goals/service.clj`
- `src/cointrail/goals/repository.clj`
- `src/cointrail/goals/spec.clj`
- `test/cointrail/goals/handlers_test.clj`
- Modify: `src/cointrail/routes.clj`, `src/cointrail/swagger.clj`

---

## Feature 4: Recurring Transactions

Rules that automatically generate transactions on a schedule via a background job.

### Data Model

**Table `recurring_transactions` (migration 012):**
```sql
CREATE TABLE IF NOT EXISTS recurring_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    type VARCHAR(7) NOT NULL CHECK (type IN ('income', 'expense')),
    amount DECIMAL(12,2) NOT NULL CHECK (amount > 0),
    description VARCHAR(255) NOT NULL,
    frequency VARCHAR(10) NOT NULL CHECK (frequency IN ('daily', 'weekly', 'monthly', 'yearly')),
    start_date DATE NOT NULL,
    end_date DATE,
    next_due_date DATE NOT NULL,
    last_generated_date DATE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);
```

**Index (migration 013):**
```sql
CREATE INDEX IF NOT EXISTS idx_recurring_next_due ON recurring_transactions(next_due_date) WHERE active = true;
```

### Endpoints

| Method | Path | Body | Description |
|--------|------|------|-------------|
| GET | `/api/recurring` | — | List user's recurring rules. Query: `?active=true\|false` |
| POST | `/api/recurring` | `{category_id, type, amount, description, frequency, start_date, end_date?}` | Create rule |
| PUT | `/api/recurring/:id` | `{category_id?, amount?, description?, end_date?}` | Update rule (does not alter already-generated transactions) |
| DELETE | `/api/recurring/:id` | — | Soft delete: sets `active = false` |
| POST | `/api/recurring/:id/skip` | — | Skip next occurrence (advances `next_due_date`) |

### Scheduled Job

**Implementation:** `java.util.concurrent.ScheduledExecutorService` with a single-thread pool, running every 1 hour.

**Processing logic (per execution cycle):**
1. Query: `SELECT * FROM recurring_transactions WHERE next_due_date <= CURRENT_DATE AND active = true`
2. For each rule, loop while `next_due_date <= today`:
   a. Create transaction in `transactions` table with the rule's fields and `date = next_due_date`
   b. Set `notes` to `"Auto-generated from recurring: {description}"`
   c. Calculate next due date based on frequency:
      - `daily`: `next_due_date.plusDays(1)`
      - `weekly`: `next_due_date.plusDays(7)`
      - `monthly`: `next_due_date.plusMonths(1)`
      - `yearly`: `next_due_date.plusYears(1)`
   d. Update `next_due_date` and `last_generated_date` on the rule
   e. If `end_date` is set and new `next_due_date > end_date`, set `active = false`
3. Each rule processed within a DB transaction (atomicity)

**Resiliency:** If the app was down for multiple days, on restart the job catches up by processing all overdue dates (the while loop). Generated transactions receive the occurrence date, not the processing date.

### Integration

- Generated transactions are normal `transactions` rows — they appear in listings, reports, and budget calculations
- The job starts in `cointrail.core/-main` alongside the Jetty server
- The job shuts down gracefully on JVM shutdown via `.shutdown` on the executor

### Scheduler Module

`src/cointrail/scheduler.clj`:
- `start!` — creates `ScheduledExecutorService`, schedules the job at fixed rate (1 hour), returns the executor
- `stop!` — calls `.shutdown` on the executor
- `process-recurring!` — the job function, called by the scheduler and also available for manual/test invocation

### Validation Rules

- `start_date`: required, must be today or future on creation
- `end_date`: optional, must be after `start_date` if provided
- `frequency`: required, one of `daily`, `weekly`, `monthly`, `yearly`
- `next_due_date` is set to `start_date` on creation (not user-provided)
- `type` and `frequency` cannot be changed via PUT (would break consistency of generated transactions)
- Empty body on PUT returns 400 "at least one field required"

### Files

- `src/cointrail/recurring/handlers.clj`
- `src/cointrail/recurring/service.clj`
- `src/cointrail/recurring/repository.clj`
- `src/cointrail/recurring/spec.clj`
- `src/cointrail/scheduler.clj`
- `test/cointrail/recurring/handlers_test.clj`
- `test/cointrail/scheduler_test.clj`
- Modify: `src/cointrail/core.clj`, `src/cointrail/routes.clj`, `src/cointrail/swagger.clj`

---

## Migration Summary

| # | File | Statement |
|---|------|-----------|
| 007 | `007-create-tags.up.sql` | CREATE TABLE tags |
| 008 | `008-create-transaction-tags.up.sql` | CREATE TABLE transaction_tags |
| 009 | `009-create-budgets.up.sql` | CREATE TABLE budgets |
| 010 | `010-create-goals.up.sql` | CREATE TABLE goals |
| 011 | `011-create-goal-deposits.up.sql` | CREATE TABLE goal_deposits |
| 012 | `012-create-recurring-transactions.up.sql` | CREATE TABLE recurring_transactions |
| 013 | `013-index-recurring-next-due.up.sql` | CREATE INDEX on recurring_transactions |

All down migrations drop the respective table/index with `IF EXISTS`.

## Testing Strategy

Each feature gets an integration test file using `ring.mock` against the real database, following the existing test patterns:
- Register + login to get JWT token
- CRUD operations with assertions on status codes and response bodies
- Ownership isolation (user A cannot see/modify user B's data)
- Validation error cases (400 responses)
- Edge cases specific to each feature (e.g., over-budget status, goal reaching target, recurring catch-up)

The scheduler gets a dedicated test that calls `process-recurring!` directly (not the scheduled executor) to verify transaction generation and date advancement logic.

## Swagger Updates

All new endpoints added to the OpenAPI 2.0 spec in `src/cointrail/swagger.clj` with request/response schemas and JWT Bearer auth documentation.
