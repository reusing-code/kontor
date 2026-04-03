# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Prerequisites

- **Go 1.22+** — https://go.dev/dl/
- **Bun** — JavaScript runtime/package manager: https://bun.sh
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **Docker + Docker Compose** (optional) — For containerized deployment

## Commands (from repo root)

- `task dev` — Start backend (Air) + frontend (Vite) concurrently
- `task build` — Build Docker image
- `task up` / `task down` — Docker Compose up/down

See `frontend/AGENTS.md` and `backend/AGENTS.md` for per-project commands.

## Project structure

- `frontend/` — React 19 + TypeScript SPA (Vite, TanStack Router/Query, shadcn/ui)
- `backend/` — Go API server (stdlib net/http, BadgerDB, slog, Prometheus)
- `Dockerfile` — Multi-stage build: frontend (Bun) → backend (Go) → Alpine runtime
- `docker-compose.yml` — Single-service deployment with named volume for DB

## Architecture overview

The app is a multi-module personal finance manager. Currently two modules exist:

- **Contracts** — Recurring subscriptions with renewal tracking, notice periods, and email reminders
- **Purchases** — One-time purchases with item details, dealer info, and document links

Each module has its own categories stored under separate DB key prefixes. Categories are module-scoped via the API route (`/api/v1/modules/{module}/categories`), not via a field on the Category model.

### DB key schema

- Contract categories: `u/{userId}/mod/contracts/cat/{categoryId}`
- Purchase categories: `u/{userId}/mod/purchases/cat/{categoryId}`
- Contracts: `u/{userId}/con/{contractId}`
- Contract category index: `u/{userId}/idx/cat_con/{catId}/{conId}`
- Purchases: `u/{userId}/pur/{purchaseId}`
- Purchase category index: `u/{userId}/idx/cat_pur/{catId}/{purId}`
- Schema version: `_meta/schema_version` (current: 2)

### Frontend routes

| Route | Purpose |
|-------|---------|
| `/` | Homepage with overview cards for all modules |
| `/contracts` | Contracts dashboard |
| `/contracts/categories/$categoryId` | Contract category detail |
| `/contracts/upcoming-renewals` | Upcoming renewals |
| `/purchases` | Purchases dashboard |
| `/purchases/categories/$categoryId` | Purchase category detail |

Routes use TanStack Router file-based conventions with dots for nesting (e.g. `contracts.index.tsx`, `contracts.categories.$categoryId.tsx`). All routes use `rootRoute` as parent with full paths (flat structure).

## Git workflow

All development must be done on feature branches. Never commit directly to `main`. Create a descriptive branch (e.g. `feat/add-export`, `fix/renewal-date-calc`) before making changes.

## Dev workflow

`task dev` from root starts both services. Vite dev server (port 5173) proxies `/api/*`, `/healthz`, `/readyz`, `/metrics` to the Go backend (port 8080). Open http://localhost:5173 in the browser.
