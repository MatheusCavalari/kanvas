# Kanvas

A real-time collaborative Kanban board — a portfolio project built to demonstrate clean architecture, tested code, and a real deployment pipeline.

- **Backend:** Go (chi, sqlc + pgx, JWT auth, WebSockets) — see [`backend/README.md`](backend/README.md)
- **Frontend:** React + TypeScript (Vite, Tailwind, React Router, Zustand) — see [`frontend/README.md`](frontend/README.md)

## Status

Phases 1-7 complete: backend foundation, authentication, boards & members, columns & cards, realtime WebSocket, a full frontend (auth, board list, kanban view with drag-and-drop, members panel, realtime sync), Playwright end-to-end tests, and automatic deployment to Fly.io on every merge to `master`. See [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for implementation plans by phase.

## Deploy

Backend and frontend each deploy to their own Fly.io app (`kanvas-backend`, `kanvas-frontend`) automatically on every push to `master`, via [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml). See [`docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md`](docs/superpowers/specs/2026-08-12-kanvas-phase7-e2e-deploy-design.md) for the one-time Fly.io account setup this depends on.
