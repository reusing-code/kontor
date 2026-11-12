# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Prerequisites

- **Go 1.27+** — https://go.dev/dl/. The version is pinned by the `go` line in `backend/go.mod`; the CI image and the Docker build read it from there
- **Bun 1.4.4** — JavaScript runtime/package manager: https://bun.sh. The version is pinned in `frontend/package.json` (`engines.bun`, an exact `X.Y.Z` since it is also the Docker tag); the CI image and the Docker build read it from there
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **Docker + Docker Compose** (optional) — For containerized deployment

## Commands (from repo root)

- `task dev` — Start backend (Air) + frontend (Vite) concurrently
- `task dev:clean-db` — Delete the local dev database (`backend/data`)
- `task ci` — Run the local backend + frontend checks that must pass before committing
- `task ci:backend` — Run backend test + lint checks
- `task ci:frontend` — Run frontend lint + build checks
- `task build` — Build Docker image (passes `BUN_VERSION` from `frontend/package.json` and `GO_VERSION` from `backend/go.mod`; a plain `docker build` needs both as `--build-arg`)
- `task up` / `task down` — Docker Compose up/down

See `frontend/AGENTS.md` and `backend/AGENTS.md` for per-project commands.

## Project structure

- `frontend/` — React 19 + TypeScript SPA (Vite, TanStack Router/Query, shadcn/ui)
- `backend/` — Go API server (stdlib net/http, BadgerDB, slog, Prometheus)
- `Dockerfile` — Multi-stage build: frontend (Bun) → backend (Go) → Alpine runtime
- `compose.yml` — Single-service deployment with named volume for DB
- `.forgejo/workflows/ci.yml` — CI (Forgejo Actions), see below
- `ci/` — CI image (`ci/Dockerfile`) and helper scripts (`ci/versions`, `ci/retry`)
- `.tool-versions` — golangci-lint version pin (asdf/mise format; mise is not required)

## CI

Tool versions have one source each: Go in `backend/go.mod`, Bun in `engines.bun` (`frontend/package.json`), golangci-lint in `.tool-versions`. `ci/versions` reads them and derives the CI image tag (`go<go>-bun<bun>-lint<lint>-<sha256 of ci/Dockerfile, 7 chars>`).

- `ci-image` runs in `docker:<ver>-cli` and pushes `forgejo.reusing.dev/tobi/kontor-ci:<tag>` only if the tag is missing (needs the `PACKAGES_TOKEN` secret, a PAT with `write:package`; the automatic job token cannot push packages). Docker Hub is only contacted when a version or `ci/Dockerfile` changes.
- `frontend` and `backend` run inside that image with no `uses:` steps: a shallow `git fetch` of `$GITHUB_SHA` with the job token, `GOTOOLCHAIN=local`, dependency downloads wrapped in `ci/retry`, and caches in named volumes (`kontor-bun`, `kontor-gomod`, `kontor-gocache`, `kontor-golangci`). The runner only mounts them once `kontor-*` is in its `container.valid_volumes` (tobi/shaker#45); until then the caches are cold on every run.
- `docker` (pushes to `main` and `v*` tags only) runs in the CI image as well, with plain `docker login` / `docker buildx build` on the dind's default `docker` builder: the BuildKit cache stays in the persistent dind, no registry cache. It first pulls the base images listed by `ci/base-images` (the Dockerfile's `FROM` lines with the version build args substituted), each with a warning and fallback to the cached image if the pull fails, then builds without `--pull`. Tags: `sha-<7>`, the branch name, and for `vX.Y.Z` tags `X.Y.Z`, `X.Y`, `latest` (pre-release tags only get their version). The image is pushed to `forgejo.reusing.dev/tobi/kontor` (`PACKAGES_TOKEN`).
- `mirror-ghcr` copies that digest unchanged to `ghcr.io/reusing-code/kontor` with the same tags (`docker buildx imagetools create --prefer-index=false` in `ci/retry`, then checks the digest; `GHCR_TOKEN`, user `reusing-code`). `compose.yml` and the deployment pull from ghcr.io, so the job is required; if ghcr.io is down, re-run only this job later.

To bump a tool, change its source; the next run builds a new CI image. Change `ci/Dockerfile` (e.g. the `DOCKER_VERSION` default) for anything else in the image.

## Architecture overview

The app ("Kontor") is a multi-module personal finance manager. Modules are first-class: each backend module owns its routes, store, models, migrations, and export section (`backend/internal/modules/{id}/`), and each frontend module owns its routes, components, hooks, and repository (`frontend/src/modules/{id}/`), described by a registry. Users can enable/disable modules per account in settings; disabled modules are hidden in the UI and their API routes return 403 (data is kept). Currently four modules exist:

- **Contracts** — Recurring subscriptions with renewal tracking, notice periods, and email reminders
- **Purchases** — One-time purchases with item details, dealer info, and document links
- **Auto** — Vehicle management with cost tracking (service, fuel, insurance, tax, tires, mileage, misc) and total cost of ownership projections
- **Ledger** — Bank account and transaction tracking with CSV import, review queue, category matching, cross references, explicit internal transfer linking between tracked accounts, and email-order enrichment from IMAP folder scans (LLM extraction, manual or scheduled in the background) or uploaded `.eml` messages

Contracts and purchases share the item-category machinery (`backend/internal/categories`), scoped via the API route (`/api/v1/modules/{module}/categories`). The Auto module uses its own vehicle/cost key structure instead of categories. The Ledger module has its own hierarchical category type. Cross-module links between ledger transactions and contract/purchase/vehicle items go through the link registry (`backend/internal/storage/link`) so modules never import each other.

An optional cross-cutting **paperless-ngx integration** (`backend/internal/paperless/`, `frontend/src/components/paperless/`) lets each user connect their paperless-ngx instance (base URL + API token, configured on the settings page). Entities (contracts, purchases, vehicles, cost entries, ledger transactions) can then attach paperless documents via a search dialog; Kontor stores the links centrally (outside the modules) and best-effort writes a back-link into a "Kontor" URL-type custom field on the paperless document. All paperless traffic is proxied through the backend; the UI hides all paperless surfaces until the integration is configured.

An optional server-wide **LLM endpoint** (`backend/internal/llm/`) gives modules structured extraction from text: one OpenAI-compatible endpoint configured by the operator (`LLM_BASE_URL`, `LLM_MODEL`, `LLM_API_KEY`, `LLM_TIMEOUT`), not per user. Modules receive an `llm.Extractor` (DI) and call `Extract`/`llm.ExtractAs[T]` with a JSON schema derived from a Go struct (OpenAI strict mode); with `LLM_BASE_URL` unset the extractor is `llm.Disabled` and returns `llm.ErrDisabled`, so callers never nil-check. The frontend learns whether LLM features exist from `GET /api/v1/capabilities` (`useCapabilities()`) and must hide LLM surfaces when `llm` is false. Prompts and model output are user data: never log them or put them into error messages. The shared **classifier** (`backend/internal/extraction/`) turns text into a proposed purchase, contract or vehicle cost with one call (enabled modules only, their categories and vehicles with IDs, today's date) and is used by the paperless pipeline and quick add. The ledger **email-order extraction** (`backend/internal/modules/ledger/email_*.go`, operations `ledger.email_order` and `ledger.email_preclassify`) is a third consumer that calls `llm.ExtractAs` directly: per message only sender, subject, date and the plain-text body (HTML stripped, capped at 12 000 characters) are sent, nothing else, and only for messages of the account's configured IMAP folder and only when the LLM is enabled; an optional per-account pre-classification sends only sender and subject of a batch (up to 40 messages) first. Known messages (order exists or marked as processed) are never sent again. Kontor stores amounts in EUR only (email orders are the exception: they keep the extracted currency and are not converted): the model reports the currency, and non-EUR amounts are converted at the ECB reference rate (`backend/internal/fx/`, Frankfurter API, `FX_RATES_URL`, `off` disables) with the original kept in the entity's comments; without a rate the amount is dropped.

**Quick add** (`backend/internal/quickadd/`, `frontend/src/components/quick-add/`): a header button (and Ctrl/Cmd+K) sends a typed note to `POST /api/v1/quick-add`, which returns a proposal without storing anything. A confident proposal with a valid category/vehicle opens the module's prefilled create dialog directly; otherwise the user picks the type and category/vehicle first (another type re-asks the LLM with that type forced). Shown only with the `llm` capability and at least one of purchases/contracts/auto enabled.

A central, source-agnostic **draft inbox** (`backend/internal/drafts/`, frontend `/inbox`) holds proposed entities (purchase, contract, auto cost, vehicle, contract/purchase/ledger category, ledger account, or an unclassified `none` draft; only the first three are ever proposed by the classifier, the others are for agents and have no parent except the ledger category's optional one) with their source (`paperless`, later `email`, `text`, `mcp`). Nothing is created without confirmation: on Accept the frontend opens the module's prefilled create dialog, creates the entity through the regular (gated) module endpoint and then calls `POST /api/v1/drafts/{id}/accept` with the new entity; the backend marks the draft accepted and runs the source's accept hook. Sources register `drafts.SourceHooks` (accept/reject) in `internal/server`; the drafts package never imports modules or sources. The first source is the **paperless pipeline** (`backend/internal/paperless/pipeline.go`): every `PAPERLESS_PIPELINE_INTERVAL` (default `15m`, `0` disables; off without an LLM) it looks at documents carrying the user's inbox tag (default `kontor-inbox`) for users who enabled it in settings, classifies their OCR content with the shared classifier (operation `paperless.classify`) offering only enabled modules with their categories and vehicles, and stores a draft (max 10 new per user and pass). Accepting stores the paperless link, writes the back-link and replaces the inbox tag with the done tag (default `kontor`, "linked to Kontor"; also set on manual attach and removed when the last link to a document goes); rejecting removes the inbox tag. Deciding a draft frees its source, so re-tagging a document creates a new draft. The inbox is shown only when the `llm` capability is on and paperless is configured.

**Personal access tokens** (`backend/internal/pat/`): users create long-lived, revocable tokens (`kpat_…`, shown once, only the SHA-256 hash is stored) in the settings API (`/api/v1/tokens`, JWT only) with scopes `read`, `drafts` and `links`, an optional expiry and at most 50 per user. Tokens are not exported and not revoked on a password change. They are also accepted on the REST API next to the JWT, restricted per route by a default-deny scope table (`backend/internal/server/scopes.go`): `read` for every `GET` (including the exports), `drafts` for the draft decisions, `links` for attaching paperless documents and adding ledger transaction references; everything else, token management and the password change need the JWT.

**MCP endpoint** (`backend/internal/mcptool/`, `backend/internal/mcpserver/`, `internal/server/mcp.go`): `/mcp` serves the Model Context Protocol (official Go SDK, stateless streamable HTTP) for agents. It accepts personal access tokens only (a JWT gets 401 with `WWW-Authenticate: Bearer`). Agents get a drafts-only tool set: list reference data, search, create drafts in the inbox and link paperless documents; they cannot create or change entities. Each request builds its own server bound to the token's user and scopes, so revoking a token, scope limits and module toggles apply on the next request. Modules contribute tools through the module registry (`module.ToolProvider`). The paperless back-link written by `attach_paperless_documents` needs the operator to set `PUBLIC_URL`.

### DB key schema

All module data lives under one prefix per module (`u/{userId}/mod/{module}/...`), so a module's footprint is a single prefix scan.

- Users: `usr/{userId}`
- User email index: `usr_email/{email}`
- User settings: `u/{userId}/settings` (includes `disabledModules`)
- Paperless integration: `u/{userId}/paperless/config` (base URL, encrypted API token, cached custom field ID), `u/{userId}/paperless/pipeline` (draft pipeline settings: `enabled`, `inboxTag`, `doneTag`; not exported) and `u/{userId}/paperless/link/{entityType}/{entityId}/{documentId}` (entityType: `contract`, `purchase`, `vehicle`, `cost`, `transaction`); links appear as an optional `paperless` section in full exports (the config/token is never exported)
- Draft inbox: `u/{userId}/drafts/item/{id}` (draft) and `u/{userId}/drafts/src/{kind}/{ref}` → draft ID (dedupes drafts per source while pending; deleted when the draft is accepted or rejected); not exported
- Personal access tokens: `u/{userId}/pat/{id}` (token record with name, scopes, expiry, `lastUsedAt` and the SHA-256 hash) and `pat_hash/{hash}` → `{userId, id}` (lookup index); the raw token is never stored; outside the module prefixes and not exported. There is no account-deletion path today; a future one must delete `u/{userId}/pat/*` and the matching `pat_hash/{hash}` keys, which live outside the user prefix, so a prefix delete would leave orphaned hashes (they fail closed, but still leak hashes).
- Contracts module: `u/{userId}/mod/contracts/` — `cat/{id}`, `con/{id}`, `idx/cat_con/{catId}/{conId}`
- Purchases module: `u/{userId}/mod/purchases/` — `cat/{id}`, `pur/{id}`, `idx/cat_pur/{catId}/{purId}`
- Auto module: `u/{userId}/mod/auto/` — `veh/{id}`, `cost/{id}`, `idx/veh_cost/{vehId}/{costId}`
- Ledger module: `u/{userId}/mod/ledger/` — `acc/{id}`, `cat/{id}`, `txn/{id}`, `imp/{id}`, `emailacc/{id}`, `eord/{id}`, and indexes `idx/acc_iban/`, `idx/acc_txn/`, `idx/txn_fp/`, `idx/imp_txn/`, `idx/file_hash/`, `idx/emailacc_eord/`, `idx/eord_msgid/` (dedup key → order; the key is the Message-ID, `mid:<hash>` for an overlong or control-character Message-ID, or `nomid:<hash of the header block>` without one), `idx/erej/{accountId}/{key}` (refusal counter of the IMAP server for a message, deleted with the account, not exported), `idx/eseen/{accountId}/{dedupKey}` (messages processed without an order: not an order, pre-classified out, too large or repeatedly unreadable; per account, deleted with the account, not exported)
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
