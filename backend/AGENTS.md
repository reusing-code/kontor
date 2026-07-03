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

**Modules:** Each feature module implements the `module.Module` interface (`internal/module`): `ID`, `Prefix` (its DB key prefix), `Migrations`, `RegisterRoutes`, `Seed`, `StartBackground`, `IsEmpty`, `ExportSection`, `ImportSection`, `PruneDeadLinks`. The server builds all modules, registers them in a `module.Registry` (order matters — it is also the import order), runs their migrations, and registers their routes through `module.Router`, which wraps every handler in the enablement gate. Users can disable modules in settings (`disabledModules`, stored as the disabled set so everything defaults to enabled); gated routes return `403 {"code":"module_disabled"}`. Data of disabled modules is kept.

**Cross-module links:** Ledger transactions reference contract/purchase/vehicle items, and those items track linked transaction IDs. The synchronization goes through `internal/storage/link.Registry` inside a single badger transaction: contracts/purchases/auto stores implement `link.Target` (AddLink/RemoveLink/Exists), the ledger store implements `link.TransactionSide`. Modules never import each other; wiring happens in `internal/server`.

**Storage:** BadgerDB (embedded LSM-tree KV store) behind `storage.Engine` (`internal/storage`), shared by per-module stores. Data is stored as JSON documents; every module's data lives under `u/{userId}/mod/{moduleId}/...` (see the root `AGENTS.md` for the full key schema). Sentinel errors (`ErrNotFound`, `ErrConflict`, ledger-specific ones) live in `internal/storage`.

**Migrations:** Per module (`internal/storage/migration`): `migration.RunModule` applies a module's pending migrations against its own `_meta/schema/{moduleId}` version key at startup. A module's migrations must only touch keys under its own prefix.

**Export/import:** Versioned envelope (`{"format":"kontor-export","formatVersion":2,...}`) with one section per module, each stamped with the module's section schema version. Full and per-module endpoints; import requires the affected modules to be empty and enabled, preserves IDs verbatim, strips ledger references to items that were not imported, and prunes dead transaction links (with warnings). Orchestrated in `internal/core/export.go`; each module owns its section shape.

**Backups:** Optional periodic full BadgerDB snapshots (`internal/storage/backup.go`) to `BACKUP_DIR` every `BACKUP_INTERVAL` (default `24h`), keeping the `BACKUP_KEEP` newest files (default 7). Empty `BACKUP_DIR` disables. Restore snapshots with the `badger` CLI (`badger restore`) or `badger.DB.Load`.

**Config:** Environment variables via `caarlos0/env` struct tags. See `.env.example` for all options.

**Logging:** `log/slog` — JSON handler in production, text in dev.

**Metrics:** Prometheus via `/metrics` endpoint. Tracks `http_requests_total`, `http_request_duration_seconds`, `http_active_requests`.

**Middleware chain (outermost first):** RequestID → Recovery → Metrics → Logging → CORS → handler. Module routes additionally pass the enablement gate after auth.

**Auth:** JWT-based authentication. Registration and login seed default data for every enabled module (idempotent; re-enabling a module in settings re-seeds it). Stores accept `userID` as a parameter; handlers extract it from auth context. Passwords must be at least 8 characters (register and change). `/auth/login` and `/auth/register` share a per-IP token-bucket rate limit (`internal/middleware/ratelimit.go`): `AUTH_RATE_LIMIT` requests (default 10) per `AUTH_RATE_WINDOW` (default `1m`), 0 disables; set `TRUST_PROXY=true` behind a reverse proxy so client IPs come from `X-Real-IP`/`X-Forwarded-For`.

## Key directories

- `cmd/server/` — Entrypoint (opens the engine, starts the server)
- `internal/config/` — Environment-based configuration
- `internal/storage/` — Badger engine, JSON txn helpers, sentinel errors, backups; `migration/` (per-module runner), `link/` (cross-module link registry)
- `internal/module/` — Module interface, registry, gating router/middleware, import result
- `internal/core/` — Users, settings (incl. enabled modules), auth/settings/health handlers, export/import orchestrator
- `internal/categories/` — Shared item-category machinery for contracts and purchases (model, store with pluggable delete cascades, handlers)
- `internal/modules/contracts/` — Contracts module incl. renewal reminder scheduler and batch item import
- `internal/modules/purchases/` — Purchases module
- `internal/modules/auto/` — Vehicles and cost entries
- `internal/modules/ledger/` — Ledger module: accounts, hierarchical categories, transactions, CSV import (`import_*.go`, comdirect/DKB providers), email-order enrichment (`email_*.go`, IMAP scan scheduler via `LEDGER_EMAIL_SCAN_INTERVAL`, default `6h`, `0` disables), keyword categorization
- `internal/httputil/` — Shared JSON response helpers
- `internal/middleware/` — Request ID, recovery, metrics, logging, CORS, auth, rate limiting
- `internal/email/` — SMTP client
- `internal/cryptoutil/` — Email password encryption
- `internal/server/` — Module wiring, mux setup, middleware, graceful shutdown, SPA serving
- `internal/version/` — Build version info

## API

All endpoints under `/api/v1/`. JSON request/response with camelCase field names.

- `POST /api/v1/auth/register` — Register user
- `POST /api/v1/auth/login` — Login (returns JWT)
- `GET|POST /api/v1/modules/{module}/categories` — List/create categories (module: `contracts` or `purchases`; gated on module enablement)
- `GET|PUT|DELETE /api/v1/modules/{module}/categories/{id}` — Get/update/delete category (delete cascades to module items)
- `GET|POST /api/v1/categories/{id}/contracts` — List/create contracts in category
- `GET /api/v1/contracts` — List all contracts
- `GET|PUT|DELETE /api/v1/contracts/{id}` — Get/update/delete contract
- `GET /api/v1/contracts/upcoming-renewals` — Upcoming renewals
- `POST /api/v1/contracts/import` — Batch JSON import
- `GET /api/v1/contracts/summary` — Contract dashboard stats
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
- `POST /api/v1/ledger/email-accounts/{emailAccountId}/test` — Test the configured IMAP connection using stored credentials
- `POST /api/v1/ledger/email-accounts/{emailAccountId}/scan` — Scan the configured IMAP inbox for matching email importers and auto-link orders; multipart `.eml` upload is also supported as a fallback
- `GET /api/v1/ledger/email-orders` — List parsed ledger email orders
- `GET /api/v1/ledger/email-orders/{emailOrderId}` — Get parsed ledger email order
- `POST /api/v1/ledger/email-orders/{emailOrderId}/link` — Manually link parsed email order to ledger transactions
- `POST /api/v1/ledger/email-orders/{emailOrderId}/reject` — Reject parsed email order
- `GET /api/v1/ledger/email-importers` — List supported email importers
- `GET /api/v1/export` — Download all user data (v2 envelope with per-module sections)
- `GET /api/v1/modules/{module}/export` — Download a single module's data
- `POST /api/v1/import` — Import an export file (affected modules must be empty and enabled; preserves IDs; email passwords must be re-entered)
- `POST /api/v1/modules/{module}/import` — Import a single module's section from an export file
- `GET /api/v1/modules` — List available modules and their enabled state
- `GET /api/v1/settings` — Get settings (renewal preferences, enabled modules)
- `PUT /api/v1/settings` — Update settings (`enabledModules` optional; omit to leave unchanged)
- `PUT /api/v1/settings/password` — Change password
- `GET /healthz` — Liveness probe
- `GET /readyz` — Readiness probe (checks DB)
- `GET /metrics` — Prometheus metrics

Linked internal transfers are intentionally protected from normal category assignment until the user explicitly unlinks them.
