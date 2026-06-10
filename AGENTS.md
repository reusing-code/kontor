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

The app is a multi-module personal finance manager. Currently four modules exist:

- **Contracts** — Recurring subscriptions with renewal tracking, notice periods, and email reminders
- **Purchases** — One-time purchases with item details, dealer info, and document links
- **Auto** — Vehicle management with cost tracking (service, fuel, insurance, tax, inspection, tires, mileage, misc) and total cost of ownership projections
- **Ledger** — Bank account and transaction tracking with CSV import, review queue, category matching, cross references, explicit internal transfer linking between tracked accounts, and email-order enrichment from IMAP inbox scans (manual or scheduled in the background) or uploaded `.eml` messages

Each module has its own categories stored under separate DB key prefixes. Categories are module-scoped via the API route (`/api/v1/modules/{module}/categories`), not via a field on the Category model. The Auto module uses its own vehicle/cost key structure instead of categories. The Ledger module has its own account, category, transaction, and import-batch keys.

### DB key schema

- Users: `usr/{userId}`
- User email index: `usr_email/{email}`
- User settings: `u/{userId}/settings`
- Contract categories: `u/{userId}/mod/contracts/cat/{categoryId}`
- Purchase categories: `u/{userId}/mod/purchases/cat/{categoryId}`
- Contracts: `u/{userId}/con/{contractId}`
- Contract category index: `u/{userId}/idx/cat_con/{catId}/{conId}`
- Purchases: `u/{userId}/pur/{purchaseId}`
- Purchase category index: `u/{userId}/idx/cat_pur/{catId}/{purId}`
- Vehicles: `u/{userId}/veh/{vehicleId}`
- Cost entries: `u/{userId}/cost/{costEntryId}`
- Vehicle cost index: `u/{userId}/idx/veh_cost/{vehicleId}/{costEntryId}`
- Ledger accounts: `u/{userId}/led/acc/{accountId}`
- Ledger account IBAN index: `u/{userId}/idx/led_acc_iban/{iban}`
- Ledger categories: `u/{userId}/led/cat/{categoryId}`
- Ledger transactions: `u/{userId}/led/txn/{transactionId}`
- Ledger account transaction index: `u/{userId}/idx/led_acc_txn/{accountId}/{bookingDate}/{transactionId}`
- Ledger transaction fingerprint index: `u/{userId}/idx/led_txn_fp/{fingerprint}`
- Ledger imports: `u/{userId}/led/imp/{batchId}`
- Ledger import transaction index: `u/{userId}/idx/led_imp_txn/{batchId}/{transactionId}`
- Ledger file hash index: `u/{userId}/idx/led_file_hash/{sha256}`
- Ledger email accounts: `u/{userId}/led/emailacc/{emailAccountId}`
- Ledger email orders: `u/{userId}/led/eord/{emailOrderId}`
- Ledger email account order index: `u/{userId}/idx/led_emailacc_eord/{emailAccountId}/{emailOrderId}`
- Ledger email message index: `u/{userId}/idx/led_eord_msgid/{messageId}`
- Schema version: `_meta/schema_version` (current: 4)

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

Routes use TanStack Router file-based conventions with dots for nesting (e.g. `contracts.index.tsx`, `contracts.categories.$categoryId.tsx`). All routes use `rootRoute` as parent with full paths (flat structure).

## Git workflow

All development must be done on feature branches. Never commit directly to `main`. Create a descriptive branch (e.g. `feat/add-export`, `fix/renewal-date-calc`) before making changes.

## Dev workflow

`task dev` from root starts both services. Vite dev server (port 5173) proxies `/api/*`, `/healthz`, `/readyz`, `/metrics` to the Go backend (port 8080). Open http://localhost:5173 in the browser.

Before committing, run `task ci` from the repo root. If you only changed one side, you may additionally run the narrower `task ci:backend` or `task ci:frontend` while iterating, but the full root `task ci` is the required pre-commit check.
