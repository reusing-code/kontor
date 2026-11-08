# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Prerequisites

- **Go 1.27+** — https://go.dev/dl/
- **Bun 1.4** — JavaScript runtime/package manager: https://bun.sh. The version is pinned in `frontend/package.json` (`engines.bun`, plain `X.Y` since it is also the Docker tag); CI (`setup-bun`) and the Docker build read it from there
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **Docker + Docker Compose** (optional) — For containerized deployment

## Commands (from repo root)

- `task dev` — Start backend (Air) + frontend (Vite) concurrently
- `task dev:clean-db` — Delete the local dev database (`backend/data`)
- `task ci` — Run the local backend + frontend checks that must pass before committing
- `task ci:backend` — Run backend test + lint checks
- `task ci:frontend` — Run frontend lint + build checks
- `task build` — Build Docker image (passes `BUN_VERSION` from `frontend/package.json`; a plain `docker build` needs `--build-arg BUN_VERSION=<version>`)
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
- **Auto** — Vehicle management with cost tracking (service, fuel, insurance, tax, tires, mileage, misc) and total cost of ownership projections
- **Ledger** — Bank account and transaction tracking with CSV import, review queue, category matching, cross references, explicit internal transfer linking between tracked accounts, and email-order enrichment from IMAP inbox scans (manual or scheduled in the background) or uploaded `.eml` messages

Contracts and purchases share the item-category machinery (`backend/internal/categories`), scoped via the API route (`/api/v1/modules/{module}/categories`). The Auto module uses its own vehicle/cost key structure instead of categories. The Ledger module has its own hierarchical category type. Cross-module links between ledger transactions and contract/purchase/vehicle items go through the link registry (`backend/internal/storage/link`) so modules never import each other.

An optional cross-cutting **paperless-ngx integration** (`backend/internal/paperless/`, `frontend/src/components/paperless/`) lets each user connect their paperless-ngx instance (base URL + API token, configured on the settings page). Entities (contracts, purchases, vehicles, cost entries, ledger transactions) can then attach paperless documents via a search dialog; Kontor stores the links centrally (outside the modules) and best-effort writes a back-link into a "Kontor" URL-type custom field on the paperless document. All paperless traffic is proxied through the backend; the UI hides all paperless surfaces until the integration is configured.

An optional server-wide **LLM endpoint** (`backend/internal/llm/`) gives modules structured extraction from text: one OpenAI-compatible endpoint configured by the operator (`LLM_BASE_URL`, `LLM_MODEL`, `LLM_API_KEY`, `LLM_TIMEOUT`), not per user. Modules receive an `llm.Extractor` (DI) and call `Extract`/`llm.ExtractAs[T]` with a JSON schema derived from a Go struct (OpenAI strict mode); with `LLM_BASE_URL` unset the extractor is `llm.Disabled` and returns `llm.ErrDisabled`, so callers never nil-check. The frontend learns whether LLM features exist from `GET /api/v1/capabilities` (`useCapabilities()`) and must hide LLM surfaces when `llm` is false. Prompts and model output are user data: never log them or put them into error messages. The paperless draft pipeline is the first user.

A central, source-agnostic **draft inbox** (`backend/internal/drafts/`, frontend `/inbox`) holds proposed entities (purchase, contract, auto cost, or an unclassified `none` draft) with their source (`paperless`, later `email`, `text`, `mcp`). Nothing is created without confirmation: on Accept the frontend opens the module's prefilled create dialog, creates the entity through the regular (gated) module endpoint and then calls `POST /api/v1/drafts/{id}/accept` with the new entity; the backend marks the draft accepted and runs the source's accept hook. Sources register `drafts.SourceHooks` (accept/reject) in `internal/server`; the drafts package never imports modules or sources. The first source is the **paperless pipeline** (`backend/internal/paperless/pipeline.go`): every `PAPERLESS_PIPELINE_INTERVAL` (default `15m`, `0` disables; off without an LLM) it looks at documents carrying the user's inbox tag (default `kontor-inbox`) for users who enabled it in settings, classifies their OCR content with one LLM call (`paperless.classify`) offering only enabled modules with their categories and vehicles, and stores a draft (max 10 new per user and pass). Accepting stores the paperless link, writes the back-link and replaces the inbox tag with the done tag (default `kontor`, "linked to Kontor"; also set on manual attach and removed when the last link to a document goes); rejecting removes the inbox tag. Deciding a draft frees its source, so re-tagging a document creates a new draft. The inbox is shown only when the `llm` capability is on and paperless is configured.

### DB key schema

All module data lives under one prefix per module (`u/{userId}/mod/{module}/...`), so a module's footprint is a single prefix scan.

- Users: `usr/{userId}`
- User email index: `usr_email/{email}`
- User settings: `u/{userId}/settings` (includes `disabledModules`)
- Paperless integration: `u/{userId}/paperless/config` (base URL, encrypted API token, cached custom field ID), `u/{userId}/paperless/pipeline` (draft pipeline settings: `enabled`, `inboxTag`, `doneTag`; not exported) and `u/{userId}/paperless/link/{entityType}/{entityId}/{documentId}` (entityType: `contract`, `purchase`, `vehicle`, `cost`, `transaction`); links appear as an optional `paperless` section in full exports (the config/token is never exported)
- Draft inbox: `u/{userId}/drafts/item/{id}` (draft) and `u/{userId}/drafts/src/{kind}/{ref}` → draft ID (dedupes drafts per source while pending; deleted when the draft is accepted or rejected); not exported
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
| `/inbox` | Draft inbox (only with the `llm` capability and paperless configured) |
| `/contracts` | Contracts dashboard |
| `/contracts/categories/$categoryId` | Contract category detail |
| `/contracts/upcoming-renewals` | Upcoming renewals |
| `/purchases` | Purchases dashboard |
| `/purchases/categories/$categoryId` | Purchase category detail |
| `/auto` | Auto / vehicles dashboard |
| `/auto/vehicles/$vehicleId` | Vehicle detail with cost entries and headline stats |
| `/auto/vehicles/$vehicleId/statistics` | Vehicle statistics with charts, yearly breakdowns, and projection |
| `/ledger` | Ledger dashboard with accounts, imports, categories, and review queue |
| `/ledger/accounts/$accountId` | Ledger account detail with transactions |
| `/ledger/categories` | Ledger category tree manager |
| `/ledger/review` | Ledger review queue |
| `/ledger/transactions/$transactionId` | Ledger transaction detail with notes, references, and transfer linking |
| `/ledger/email-accounts` | Ledger email account management |
| `/ledger/email-orders` | Parsed email orders and matching status |

Module routes live in `frontend/src/modules/{id}/routes/` and are registered through the module registry; core routes (`/`, `/login`, `/settings`, `/inbox`) stay in `frontend/src/routes/`. All routes use `rootRoute` as parent with full paths (flat structure). Module routes carry a `beforeLoad` guard that redirects to `/` when the module is disabled.

## Git workflow

All development must be done on feature branches. Never commit directly to `main`. Create a descriptive branch (e.g. `feat/add-export`, `fix/renewal-date-calc`) before making changes.

Commit messages and PR descriptions must not contain AI attribution of any kind: no `Co-Authored-By: Claude` (or similar) trailers, no "Generated with Claude Code" lines, and especially no links to AI assistant sessions.

## Dev workflow

`task dev` from root starts both services. Vite dev server (port 5173) proxies `/api/*`, `/healthz`, `/readyz`, `/metrics` to the Go backend (port 8080). Open http://localhost:5173 in the browser.

Before committing, run `task ci` from the repo root. If you only changed one side, you may additionally run the narrower `task ci:backend` or `task ci:frontend` while iterating, but the full root `task ci` is the required pre-commit check.
