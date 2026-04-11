# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Prerequisites

- **Go 1.26+** — https://go.dev/dl/
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **golangci-lint** (optional) — Linter: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest` or `pacman -S golangci-lint`

## Commands

- `task dev` — Start with Air (live reload), sets `CORS_ORIGIN` for frontend dev server
- `task build` — Build the binary
- `task test` — Run tests
- `task lint` — Run golangci-lint (use a recent binary compatible with the repo Go version)

## Architecture

Go 1.26+ stdlib `net/http` with method+pattern routing. Single binary that optionally serves the frontend SPA.

**Storage:** BadgerDB (embedded LSM-tree KV store). Data stored as JSON documents with module-scoped key prefixes for multi-user namespacing:

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

**Migrations:** Version-based schema migrations in `internal/store/migration/`. V1 renamed `pricePerMonth` → `price`. V2 moved category keys from `u/{userId}/cat/{id}` to module-scoped `u/{userId}/mod/{module}/cat/{id}`.

**Config:** Environment variables via `caarlos0/env` struct tags. See `.env.example` for all options.

**Logging:** `log/slog` — JSON handler in production, text in dev.

**Metrics:** Prometheus via `/metrics` endpoint. Tracks `http_requests_total`, `http_request_duration_seconds`, `http_active_requests`.

**Middleware chain (outermost first):** RequestID → Recovery → Metrics → Logging → CORS → handler.

**Auth:** JWT-based authentication. Registration and login seed default categories for both modules. The store interface accepts `userID` as a parameter; handlers extract it from auth context.

## Key directories

- `cmd/server/` — Entrypoint
- `internal/config/` — Environment-based configuration
- `internal/model/` — Category, Contract, Purchase, Vehicle, and CostEntry types (JSON tags match frontend Zod schemas)
- `internal/store/` — Store interface + BadgerDB implementation
- `internal/store/migration/` — Schema migration registry and versioned migrations
- `internal/handler/` — HTTP handlers (auth, category CRUD, contract CRUD, purchase CRUD, vehicle CRUD, cost entry CRUD, ledger accounts/categories/transactions/import, summaries)
- `internal/ledgeremail/` — Uploaded `.eml` parsing, importer registry, and auto-linking for ledger email orders
- `internal/middleware/` — Request ID, recovery, metrics, logging, CORS, auth
- `internal/server/` — Mux setup, middleware wiring, graceful shutdown, SPA serving
- `internal/reminder/` — Email reminder scheduler for contract renewals
- `internal/version/` — Build version info

## API

All endpoints under `/api/v1/`. JSON request/response with camelCase field names.

- `POST /api/v1/auth/register` — Register user
- `POST /api/v1/auth/login` — Login (returns JWT)
- `GET|POST /api/v1/modules/{module}/categories` — List/create categories (module: `contracts` or `purchases`)
- `GET|PUT|DELETE /api/v1/modules/{module}/categories/{id}` — Get/update/delete category (delete cascades to module items)
- `GET|POST /api/v1/categories/{id}/contracts` — List/create contracts in category
- `GET /api/v1/contracts` — List all contracts
- `GET|PUT|DELETE /api/v1/contracts/{id}` — Get/update/delete contract
- `GET /api/v1/contracts/upcoming-renewals` — Upcoming renewals
- `POST /api/v1/contracts/import` — Batch JSON import
- `GET /api/v1/summary` — Contract dashboard stats
- `GET|POST /api/v1/categories/{id}/purchases` — List/create purchases in category
- `GET /api/v1/purchases` — List all purchases
- `GET|PUT|DELETE /api/v1/purchases/{id}` — Get/update/delete purchase
- `GET /api/v1/purchases/summary` — Purchase spending stats
- `GET /api/v1/vehicles` — List vehicles
- `POST /api/v1/vehicles` — Create vehicle
- `GET|PUT|DELETE /api/v1/vehicles/{id}` — Get/update/delete vehicle
- `GET /api/v1/vehicles/{id}/summary` — Vehicle cost summary + projection
- `GET /api/v1/vehicles/{id}/costs` — List cost entries for vehicle
- `POST /api/v1/vehicles/{id}/costs` — Create cost entry for vehicle
- `GET|PUT|DELETE /api/v1/costs/{id}` — Get/update/delete cost entry
- `GET /api/v1/ledger/accounts` — List ledger accounts
- `GET /api/v1/ledger/accounts/{accountId}` — Get ledger account
- `GET /api/v1/ledger/accounts/{accountId}/transactions` — List ledger account transactions
- `GET /api/v1/ledger/imports` — List ledger import batches
- `POST /api/v1/ledger/imports/preview` — Preview ledger import
- `POST /api/v1/ledger/imports/{previewId}/commit` — Commit ledger import preview
- `GET|POST /api/v1/ledger/categories` — List/create ledger categories
- `GET|PUT|DELETE /api/v1/ledger/categories/{id}` — Get/update/delete ledger category
- `GET /api/v1/ledger/transactions` — Ledger review queue
- `GET /api/v1/ledger/transactions/{transactionId}` — Get ledger transaction
- `PUT /api/v1/ledger/transactions/{transactionId}` — Update ledger transaction note, links, and references
- `GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates` — List internal transfer candidates
- `POST /api/v1/ledger/transactions/{transactionId}/transfer-link` — Link internal transfer pair
- `DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link` — Unlink internal transfer pair
- `POST /api/v1/ledger/transactions/{transactionId}/review` — Review/categorize transaction
- `GET|POST /api/v1/ledger/email-accounts` — List/create ledger email accounts
- `GET|PUT|DELETE /api/v1/ledger/email-accounts/{emailAccountId}` — Get/update/delete ledger email account
- `POST /api/v1/ledger/email-accounts/{emailAccountId}/scan` — Parse uploaded `.eml` files for matching email importers and auto-link orders
- `GET /api/v1/ledger/email-orders` — List parsed ledger email orders
- `GET /api/v1/ledger/email-orders/{emailOrderId}` — Get parsed ledger email order
- `POST /api/v1/ledger/email-orders/{emailOrderId}/link` — Manually link parsed email order to ledger transactions
- `POST /api/v1/ledger/email-orders/{emailOrderId}/reject` — Reject parsed email order
- `GET /api/v1/ledger/email-importers` — List supported email importers
- `GET /api/v1/settings` — Get renewal preferences
- `PUT /api/v1/settings` — Update renewal preferences
- `PUT /api/v1/settings/password` — Change password
- `GET /healthz` — Liveness probe
- `GET /readyz` — Readiness probe (checks DB)
- `GET /metrics` — Prometheus metrics

Linked internal transfers are intentionally protected from normal category assignment until the user explicitly unlinks them.
