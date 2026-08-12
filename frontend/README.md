# Kanvas frontend

React + TypeScript SPA for Kanvas — Vite, Tailwind CSS, React Router, Zustand.

## Prerequisites

- Node.js 20+
- The Kanvas backend running locally (see `../backend/README.md`) with CORS configured for this app's origin (`CORS_ALLOWED_ORIGIN`, defaults to `http://localhost:5173`)

## Local development

    cp .env.example .env   # defaults to VITE_API_URL=http://localhost:8080
    npm install
    npm run dev

Open the printed local URL (usually `http://localhost:5173`).

## Testing

    npm test          # Vitest + Testing Library, all fetch calls mocked
    npm run lint
    npm run build

## Authentication

- The access token lives in memory only, held in `src/api/client.ts`'s module scope — never in `localStorage`, to limit XSS exposure. The Zustand store (`src/features/auth/useAuthStore.ts`) does not hold the token itself; it tracks `user`/`status` for components to read.
- The refresh token is an `httpOnly` cookie set by the backend; the frontend never reads it directly. `src/api/client.ts` sends `credentials: "include"` on every request so the browser attaches/receives it automatically.
- On a `401`, `src/api/client.ts` transparently calls `/auth/refresh` once and retries the original request; if that also fails, the user is signed out and redirected to `/login`.

## Project layout

    src/
      api/          HTTP client (auth header injection, 401 refresh-retry) and auth API functions
      features/
        auth/        login/register pages, Zustand session store
      routes/        React Router setup, protected-route guard
      components/
        layout/      authenticated app shell (header, logout)
      lib/           env var loading, test setup
