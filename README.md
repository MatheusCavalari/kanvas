# Kanvas

A real-time collaborative Kanban board — a portfolio project built to demonstrate clean architecture, tested code, and a real deployment pipeline.

- **Backend:** Go (chi, sqlc + pgx, JWT auth, WebSockets) — see [`backend/README.md`](backend/README.md)
- **Frontend:** React + TypeScript (Vite, Tailwind, React Router, Zustand) — see [`frontend/README.md`](frontend/README.md)

## Status

Phases 1-7 complete: backend foundation, authentication, boards & members, columns & cards, realtime WebSocket, a full frontend (auth, board list, kanban view with drag-and-drop, members panel, realtime sync), Playwright end-to-end tests, and automatic deployment to Render on every merge to `master`. See [`docs/superpowers/specs/2026-08-10-kanvas-design.md`](docs/superpowers/specs/2026-08-10-kanvas-design.md) for the full design and [`docs/superpowers/plans/`](docs/superpowers/plans/) for implementation plans by phase.

## Deploy

Backend, frontend, and Postgres are defined as a single [Render Blueprint](https://render.com/docs/blueprint-spec) in [`render.yaml`](render.yaml) — `kanvas-backend` (Docker web service), `kanvas-frontend` (static site), and `kanvas-db` (managed Postgres), all on Render's free tier. Once the Blueprint is connected to this repo in the Render dashboard, it redeploys automatically on every push to `master`. The free-tier backend spins down after 15 minutes of inactivity and takes a few seconds to wake on the next request; the static frontend has no such cold start.
