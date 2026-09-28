# CoinTrail — Personal Finance REST API

## Overview

CoinTrail is a RESTful personal finance API built in Clojure. It provides JWT-authenticated endpoints for managing transactions (income/expense), custom categories, and financial reports with aggregations by period and category. The project demonstrates idiomatic functional programming on the JVM — immutability, function composition via threading macros, data-driven design with maps, and declarative validation via clojure.spec.

**Repository:** MatheusCavalari/cointrail (public)

## Tech Stack

| Layer | Technology | Details |
|-------|------------|---------|
| Runtime | Clojure 1.12, JVM 21 | Eclipse Temurin base image |
| HTTP | Ring 1.12 + Compojure | Middleware stack, routing DSL |
| Database | PostgreSQL 16 | next.jdbc + HoneySQL 2 for data access |
| Migrations | Migratus | SQL files, auto-run on boot |
| Auth | buddy-auth + buddy-hashers | JWT tokens (24h expiry), bcrypt password hashing |
| Validation | clojure.spec.alpha | Declarative specs for all request/response data |
| API Docs | ring-swagger + Swagger UI | Served at `/swagger-ui` |
| Config | environ | All config from environment variables |
| Logging | timbre | Structured logging |
| Build | Leiningen | uberjar for production |
| CI | GitHub Actions | clj-kondo lint + lein test + docker compose build |
| Infrastructure | Docker Compose | 2 services: app (:8080) + postgres (:5432) |

## Architecture

Layered architecture with clear namespace separation:

```
Request → Ring middleware stack (JSON parse → logging → CORS → JWT auth)
        → Compojure routing
        → Handler (extract params, validate with spec, call service)
        → Service (business logic, call repository)
        → Repository (build HoneySQL query → execute via next.jdbc)
        → Response map {:status 200 :body data}
```

### Principles

- **Handlers are thin** — extract data from request, validate, delegate to service, return response maps
- **Services are pure when possible** — receive data, return data
- **Repositories encapsulate all SQL** — no other namespace knows HoneySQL
- **All config from env vars** — zero hardcoded values

### Project Structure

```
cointrail/
├── src/cointrail/
│   ├── core.clj              # Entry point — starts HTTP server
│   ├── config.clj            # Env var reading via environ
│   ├── db.clj                # DataSource pool (HikariCP via next.jdbc)
│   ├── middleware.clj         # Ring middlewares (JWT auth, CORS, JSON, logging)
│   ├── routes.clj            # Compojure routes — aggregates all sub-routers
│   ├── auth/
│   │   ├── handlers.clj      # POST /register, POST /login
│   │   ├── service.clj       # Hash password, generate/verify JWT
│   │   └── spec.clj          # Validation specs (email, password)
│   ├── categories/
│   │   ├── handlers.clj      # CRUD handlers
│   │   ├── service.clj       # Business logic
│   │   ├── repository.clj    # HoneySQL queries
│   │   └── spec.clj          # Validation specs
│   ├── transactions/
│   │   ├── handlers.clj      # CRUD + filtered listing
│   │   ├── service.clj       # Business validations, calculations
│   │   ├── repository.clj    # Dynamic filter queries via HoneySQL
│   │   └── spec.clj          # Validation specs
│   └── reports/
│       ├── handlers.clj      # GET /summary, GET /by-category
│       ├── service.clj       # Aggregations and calculations
│       └── repository.clj    # GROUP BY, SUM, date range queries
├── test/cointrail/           # Mirrors src/, tests per namespace
├── resources/
│   └── migrations/           # Migratus SQL files (up/down)
├── Dockerfile
├── docker-compose.yml
└── project.clj               # Leiningen config
```

## Data Model

### users

| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| email | VARCHAR(255) | UNIQUE, NOT NULL |
| password | VARCHAR(255) | NOT NULL (bcrypt hash) |
| name | VARCHAR(100) | NOT NULL |
| created_at | TIMESTAMPTZ | DEFAULT now() |

### categories

| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| user_id | UUID | FK → users(id) ON DELETE CASCADE |
| name | VARCHAR(50) | NOT NULL |
| type | VARCHAR(7) | NOT NULL, CHECK IN ('income', 'expense') |
| icon | VARCHAR(30) | Nullable, emoji or icon name |
| is_default | BOOLEAN | DEFAULT false |
| created_at | TIMESTAMPTZ | DEFAULT now() |

UNIQUE constraint on (user_id, name, type).

### transactions

| Column | Type | Constraints |
|--------|------|-------------|
| id | UUID | PK, DEFAULT gen_random_uuid() |
| user_id | UUID | FK → users(id) ON DELETE CASCADE |
| category_id | UUID | FK → categories(id) ON DELETE SET NULL |
| type | VARCHAR(7) | NOT NULL, CHECK IN ('income', 'expense') |
| amount | DECIMAL(12,2) | NOT NULL, CHECK (amount > 0) |
| description | VARCHAR(255) | Nullable |
| date | DATE | NOT NULL, DEFAULT CURRENT_DATE |
| notes | TEXT | Nullable |
| created_at | TIMESTAMPTZ | DEFAULT now() |
| updated_at | TIMESTAMPTZ | DEFAULT now() |

### Indexes

- `transactions(user_id, date)` — listing and period reports
- `transactions(user_id, category_id)` — category reports
- `categories(user_id)` — user category listing

### Seed Categories

Created automatically on user registration:

- **Income:** Salário, Freelance, Investimentos, Outros
- **Expense:** Alimentação, Transporte, Moradia, Saúde, Lazer, Educação, Outros

## API Endpoints

### Auth (public)

| Method | Endpoint | Body | Response |
|--------|----------|------|----------|
| POST | `/api/auth/register` | `{email, password, name}` | `201 {user, token}` |
| POST | `/api/auth/login` | `{email, password}` | `200 {user, token}` |

Password requirements: minimum 8 characters. Email validated via spec regex. JWT token expires in 24 hours, payload contains `{user_id, email, exp}`.

### Categories (authenticated)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/categories` | List user categories (default + custom) |
| POST | `/api/categories` | Create custom category `{name, type, icon}` |
| PUT | `/api/categories/:id` | Update (custom only, not default) |
| DELETE | `/api/categories/:id` | Delete (custom only, not default) |

### Transactions (authenticated)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/transactions` | List with filters via query params |
| POST | `/api/transactions` | Create `{type, amount, category_id, date, description, notes}` |
| GET | `/api/transactions/:id` | Detail |
| PUT | `/api/transactions/:id` | Update |
| DELETE | `/api/transactions/:id` | Delete |

**Query params for GET /transactions:**
- `type` — filter by income/expense
- `category_id` — filter by category UUID
- `from` — start date (YYYY-MM-DD)
- `to` — end date (YYYY-MM-DD)
- `sort` — sort field (date, amount, created_at). Default: date
- `order` — asc/desc. Default: desc
- `page` — page number. Default: 1
- `per_page` — items per page. Default: 20, max: 100

All listing endpoints return paginated response:
```json
{
  "data": [...],
  "meta": {"page": 1, "per_page": 20, "total": 57, "total_pages": 3}
}
```

### Reports (authenticated)

| Method | Endpoint | Query Params | Response |
|--------|----------|-------------|----------|
| GET | `/api/reports/summary` | `from`, `to` | `{balance, total_income, total_expense, transaction_count}` |
| GET | `/api/reports/by-category` | `from`, `to`, `type` | `[{category, total, percentage, count}]` |

When `from`/`to` are omitted, defaults to current month.

### Health (public)

| Method | Endpoint | Response |
|--------|----------|----------|
| GET | `/api/health` | `{status, database, version}` |

### Error Response Format

```json
{"error": "validation_failed", "message": "...", "details": {"field": "reason"}}
```

Status codes:
- 400 — validation error
- 401 — not authenticated
- 403 — resource belongs to another user
- 404 — not found
- 409 — email already registered

## Infrastructure

### Docker Compose

Two services:
- **db** — `postgres:16-alpine` with healthcheck, persistent volume
- **app** — multi-stage build (Leiningen builder → Temurin JRE runtime)

Environment variables:
- `DATABASE_URL` — JDBC connection string
- `JWT_SECRET` — signing key for JWT tokens
- `PORT` — HTTP server port (default 8080)

### Dockerfile

Multi-stage build:
1. **Builder stage** — `clojure:temurin-21-lein`, copies project.clj first for dependency caching, builds uberjar
2. **Runtime stage** — `eclipse-temurin:21-jre-alpine`, copies standalone jar, exposes port, runs with `java -jar`

### GitHub Actions CI

Three jobs:
- **lint** — clj-kondo static analysis
- **test** — `lein test` with PostgreSQL service container
- **docker** — `docker compose build` verification

### How to Run

```bash
git clone https://github.com/MatheusCavalari/cointrail.git
cd cointrail
docker compose up --build
# API at http://localhost:8080
# Swagger UI at http://localhost:8080/swagger-ui
```

Migrations run automatically on application boot via Migratus.
