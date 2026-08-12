# Kanvas

A real-time collaborative Kanban board — a portfolio project built to demonstrate clean architecture, tested code, and a real deployment pipeline.

- **Backend:** Go (chi, sqlc + pgx, JWT auth, WebSockets) — see [`backend/README.md`](backend/README.md)
- **Frontend:** React + TypeScript (Vite, Tailwind, React Router, Zustand) — see [`frontend/README.md`](frontend/README.md)

## Status

Phases 1-5 complete: backend foundation, authentication, boards & members, columns & cards, realtime WebSocket, and the frontend's authentication flow (register/login/logout with automatic token refresh). See [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for implementation plans by phase.
