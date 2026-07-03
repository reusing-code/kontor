# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Prerequisites

- **Go 1.26+** — https://go.dev/dl/
- **Bun** — JavaScript runtime/package manager: https://bun.sh
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **Docker + Docker Compose** (optional) — For containerized deployment

## Commands (from repo root)

- `task dev` — Start backend (Air) + frontend (Vite) concurrently
- `task ci` — Run the local backend + frontend checks that must pass before committing
- `task ci:backend` — Run backend test + lint checks
- `task ci:frontend` — Run frontend lint + build checks
- `task build` — Build Docker image
- `task up` / `task down` — Docker Compose up/down

See `frontend/AGENTS.md` and `backend/AGENTS.md` for per-project commands.

## Project structure

- `frontend/` — React 19 + TypeScript SPA (Vite, TanStack Router/Query, shadcn/ui)
- `backend/` — Go API server (stdlib net/http, BadgerDB, slog, Prometheus)
- `Dockerfile` — Multi-stage build: frontend (Bun) → backend (Go) → Alpine runtime
- `compose.yml` — Single-service deployment with named volume for DB

## Architecture overview

The app ("Kontor") is a multi-module personal finance manager. Modules are first-class: each backend module owns its routes, store, models, migrations, and export section (`backend/internal/modules/{id}/`), and each frontend module owns its routes, components, hooks, and repository (`frontend/src/modules/{id}/`), described by a registry. Users can enable/disable modules per account in settings; disabled modules are hidden in the UI and their API routes return 403 (data is kept). Currently four modules exist:

- **Contracts** — Recurring subscriptions with renewal tracking, notice periods, and email reminders
- **Purchases** — One-time purchases with item details, dealer info, and document links
- **Auto** — Vehicle management with cost tracking (service, fuel, insurance, tax, inspection, tires, mileage, misc) and total cost of ownership projections
- **Ledger** — Bank account and transaction tracking with CSV import, review queue, category matching, cross references, explicit internal transfer linking between tracked accounts, and email-order enrichment from IMAP inbox scans (manual or scheduled in the background) or uploaded `.eml` messages

Contracts and purchases share the item-category machinery (`backend/internal/categories`), scoped via the API route (`/api/v1/modules/{module}/categories`). The Auto module uses its own vehicle/cost key structure instead of categories. The Ledger module has its own hierarchical category type. Cross-module links between ledger transactions and contract/purchase/vehicle items go through the link registry (`backend/internal/storage/link`) so modules never import each other.

### DB key schema

All module data lives under one prefix per module (`u/{userId}/mod/{module}/...`), so a module's footprint is a single prefix scan.

- Users: `usr/{userId}`
- User email index: `usr_email/{email}`
- User settings: `u/{userId}/settings` (includes `disabledModules`)
- Contracts module: `u/{userId}/mod/contracts/` — `cat/{id}`, `con/{id}`, `idx/cat_con/{catId}/{conId}`
- Purchases module: `u/{userId}/mod/purchases/` — `cat/{id}`, `pur/{id}`, `idx/cat_pur/{catId}/{purId}`
- Auto module: `u/{userId}/mod/auto/` — `veh/{id}`, `cost/{id}`, `idx/veh_cost/{vehId}/{costId}`
- Ledger module: `u/{userId}/mod/ledger/` — `acc/{id}`, `cat/{id}`, `txn/{id}`, `imp/{id}`, `emailacc/{id}`, `eord/{id}`, and indexes `idx/acc_iban/`, `idx/acc_txn/`, `idx/txn_fp/`, `idx/imp_txn/`, `idx/file_hash/`, `idx/emailacc_eord/`, `idx/eord_msgid/`
- Schema versions: `_meta/schema/{moduleId}` (per module, 8-byte big-endian uint64; all currently 0)

Migrations are per module: each module supplies its own `Migrations()` list, applied at startup against its own version key. A module's migrations must only touch keys under its own prefix. Pre-relaunch databases (global `_meta/schema_version`, un-namespaced keys) are not supported.

### Frontend routes

| Route | Purpose |
|-------|---------|
| `/` | Homepage with overview cards for all modules |
| `/login` | Login / registration |
| `/settings` | User settings |
| `/contracts` | Contracts dashboard |
| `/contracts/categories/$categoryId` | Contract category detail |
| `/contracts/upcoming-renewals` | Upcoming renewals |
| `/purchases` | Purchases dashboard |
| `/purchases/categories/$categoryId` | Purchase category detail |
| `/auto` | Auto / vehicles dashboard |
| `/auto/vehicles/$vehicleId` | Vehicle detail with cost entries and summary |
| `/ledger` | Ledger dashboard with accounts, imports, categories, and review queue |
| `/ledger/accounts/$accountId` | Ledger account detail with transactions |
| `/ledger/categories` | Ledger category tree manager |
| `/ledger/review` | Ledger review queue |
| `/ledger/transactions/$transactionId` | Ledger transaction detail with notes, references, and transfer linking |
| `/ledger/email-accounts` | Ledger email account management |
| `/ledger/email-orders` | Parsed email orders and matching status |

Module routes live in `frontend/src/modules/{id}/routes/` and are registered through the module registry; core routes (`/`, `/login`, `/settings`) stay in `frontend/src/routes/`. All routes use `rootRoute` as parent with full paths (flat structure). Module routes carry a `beforeLoad` guard that redirects to `/` when the module is disabled.

## Git workflow

All development must be done on feature branches. Never commit directly to `main`. Create a descriptive branch (e.g. `feat/add-export`, `fix/renewal-date-calc`) before making changes.

## Dev workflow

`task dev` from root starts both services. Vite dev server (port 5173) proxies `/api/*`, `/healthz`, `/readyz`, `/metrics` to the Go backend (port 8080). Open http://localhost:5173 in the browser.

Before committing, run `task ci` from the repo root. If you only changed one side, you may additionally run the narrower `task ci:backend` or `task ci:frontend` while iterating, but the full root `task ci` is the required pre-commit check.
