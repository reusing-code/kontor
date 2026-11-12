# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Prerequisites

- **Go 1.27+** — https://go.dev/dl/
- **Task** — Task runner (taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest` or `pacman -S go-task`
- **Air** — Live reload for Go: `go install github.com/air-verse/air@latest`
- **golangci-lint** (optional) — Linter: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0` or `pacman -S golangci-lint`; use the version CI pins in the root `.tool-versions` (`task lint` warns on a mismatch)

## Commands

- `task dev` — Start with Air (live reload), sets `CORS_ORIGIN` for frontend dev server
- `task clean-db` — Delete the local dev database (`./data`)
- `task build` — Build the binary
- `task test` — Run tests
- `task lint` — Run golangci-lint (warns if the installed version differs from `.tool-versions`)

## Architecture

Go 1.27+ stdlib `net/http` with method+pattern routing. Single binary that optionally serves the frontend SPA.

**Modules:** Each feature module implements the `module.Module` interface (`internal/module`): `ID`, `Prefix` (its DB key prefix), `Migrations`, `RegisterRoutes`, `Seed`, `StartBackground`, `IsEmpty`, `ExportSection`, `ImportSection`, `PruneDeadLinks`. The server builds all modules, registers them in a `module.Registry` (order matters — it is also the import order), runs their migrations, and registers their routes through `module.Router`, which wraps every handler in the enablement gate. Users can disable modules in settings (`disabledModules`, stored as the disabled set so everything defaults to enabled); gated routes return `403 {"code":"module_disabled"}`. Data of disabled modules is kept.

**Cross-module links:** Ledger transactions reference contract/purchase/vehicle items, and those items track linked transaction IDs. The synchronization goes through `internal/storage/link.Registry` inside a single badger transaction: contracts/purchases/auto stores implement `link.Target` (AddLink/RemoveLink/Exists), the ledger store implements `link.TransactionSide`. Modules never import each other; wiring happens in `internal/server`.

**Storage:** BadgerDB (embedded LSM-tree KV store) behind `storage.Engine` (`internal/storage`), shared by per-module stores. Data is stored as JSON documents; every module's data lives under `u/{userId}/mod/{moduleId}/...` (see the root `AGENTS.md` for the full key schema). Sentinel errors (`ErrNotFound`, `ErrConflict`, ledger-specific ones) live in `internal/storage`.

**Migrations:** Per module (`internal/storage/migration`): `migration.RunModule` applies a module's pending migrations against its own `_meta/schema/{moduleId}` version key at startup. A module's migrations must only touch keys under its own prefix.

**Export/import:** Versioned envelope (`{"format":"kontor-export","formatVersion":2,...}`) with one section per module, each stamped with the module's section schema version. Full and per-module endpoints; import requires the affected modules to be empty and enabled, preserves IDs verbatim, strips ledger references to items that were not imported, and prunes dead transaction links (with warnings). Orchestrated in `internal/core/export.go`; each module owns its section shape.

**Backups:** Optional periodic full BadgerDB snapshots (`internal/storage/backup.go`) to `BACKUP_DIR` every `BACKUP_INTERVAL` (default `24h`), keeping the `BACKUP_KEEP` newest files (default 7). Empty `BACKUP_DIR` disables. Restore snapshots with the `badger` CLI (`badger restore`) or `badger.DB.Load`.

**Config:** Environment variables via `caarlos0/env` struct tags. See `.env.example` for all options.

**LLM:** Optional server-wide OpenAI-compatible endpoint (`internal/llm`). `LLM_BASE_URL` (unset disables), `LLM_MODEL` (required once the base URL is set), `LLM_API_KEY` (optional bearer token), `LLM_TIMEOUT` (per attempt, default `60s`); partial config fails at startup. `llm.New` returns a `*llm.Client` or `llm.Disabled`; both implement `llm.Extractor` (`Extract`, `Enabled`). Requests go to `POST {base}/chat/completions` with `response_format: json_schema` (`strict: true`); schemas come from `llm.SchemaFor[T]` (every field required, only pointer fields nullable, so unknown text comes back as `""`; nullable objects and enums use `anyOf`; optional `llm:"desc=...,enum=a|b"` tag. Anthropic models allow at most 16 nullable fields per schema). Output that is not a JSON object or does not decode into `T` (`llm.ExtractAs`) is retried once; errors are `ErrDisabled`, `ErrInvalidOutput`, `ErrRefused`, `ErrTruncated` (no retry) and `*APIError` (upstream `error.message` only). Never log prompts or model output or put them into errors. Tests use `llmtest.Fake`.

**Drafts:** `internal/drafts` is the source-agnostic draft inbox (model, store, handler). A draft has `status` (`pending|accepted|rejected`), `targetType` (`purchase|contract|cost|vehicle|contract_category|purchase_category|ledger_category|ledger_account|none`), `parentId` (category for purchase/contract, vehicle for cost, optional parent category for `ledger_category`; rejected with 400 for vehicle, contract/purchase category and ledger account), `input` (the module's create-input JSON), `source{kind,ref,title,url}`, `note` (code the UI translates: `unclassified`, `empty_content`, `llm_invalid_output`, `llm_refused`, `llm_truncated`, `llm_request_rejected`, `currency_converted`, `currency_not_converted`), `result{entityType,entityId}`. `Store.Create` fails with `ErrDuplicateSource` while a draft for the same source exists; `Decide` frees the source. Sources register `SourceHooks` on the handler: `OnAccept` runs before the draft is marked accepted (error → 500, draft stays pending), `OnReject` after. The accept `entityType` must equal the draft's `targetType` (any type but `none`). `Store.Counts` backs `GET /drafts/summary`. The classifier (`extraction.Targets`) is unchanged and never proposes the new types. Accept does not verify that the entity exists.

**Extraction:** `internal/extraction` is the shared classifier. `Classifier.EnabledTargets` / `LoadCatalog` build what the model may choose from (enabled purchase/contract/cost targets, categories and vehicles with IDs; module lookups through `extraction.Deps`, wired in `internal/server/extraction.go`, so the package does not import modules); `Catalog.Only(target)` forces one type. `Classify(ctx, operation, catalog, Input{Kind: InputDocument|InputNote, Title, Text}, now)` makes one `llm.ExtractAs` call (schema `{kind, confident, purchase?, contract?, cost?}`, each extract with `currency`; prompt per input kind with today's date; text truncated to 20k characters) and returns a `Proposal{Target, ParentID, Input, Note, Confident, Currency}` whose input has the module create-input shape. Unknown category/vehicle IDs, invalid dates, intervals, URLs and cost types are dropped or defaulted. `none`, empty text and `ErrInvalidOutput`/`ErrRefused`/`ErrTruncated`/LLM 400/413/422 give a `none` proposal with a note; other LLM errors are returned (transient; `IsTimeout` for timeouts). `Confident` is the model's flag and false without a valid parent or with a foreign currency. Non-EUR amounts are converted with `fx.Rates` at the purchase/cost date (today if missing, contracts always today), rounded to cents, original appended to `comments` (`USD 640.00 → EUR 572.86 (ECB 2026-10-30, 1 USD = 0.89509 EUR)`), note `currency_converted`; without a rate (disabled, unknown code, lookup failure) the amount is dropped, comment `USD 640.00 (not converted)`, note `currency_not_converted`.

**FX rates:** `internal/fx` looks up ECB reference rates from a Frankfurter-compatible API: `GET {FX_RATES_URL}/{date}?base={from}&symbols=EUR` (default `https://api.frankfurter.dev/v1`; `off` disables, an empty value falls back to the default). Future dates are clamped to today; past-date rates are cached in memory; 10s timeout. `fx.Disabled` returns `ErrDisabled`.

**Quick add:** `internal/quickadd` handles `POST /api/v1/quick-add` with the classifier (operation `quickadd.classify`, note prompt). Stateless; the typed text is never logged.

**Paperless pipeline:** `paperless.Pipeline` (`PAPERLESS_PIPELINE_INTERVAL`, default `15m`, `0` disables; not started when the LLM is disabled). Per user with the pipeline enabled, paperless configured and at least one of purchases/contracts/auto enabled: find the inbox tag (never created by the pipeline), list tagged documents (oldest first, up to 500), skip documents that have a draft, fetch content for at most 10 new ones and classify each with the shared classifier (operation `paperless.classify`, document prompt; see Extraction). `none` proposals become `none` drafts with a note; transient LLM errors create no draft, stop the pass and are retried next time. `NewPipeline` takes the classifier and a user-ID lister (wired in `internal/server`). `paperless.Service` resolves a user's client and implements attach (links, back-link, done tag, optional tag removal) and detach (clears back-link, removes the done tag with the last link); `paperless.DraftHooks` uses it for accept (link must be stored, the rest is best effort → warnings) and reject (remove inbox tag). Tests use `paperless/paperlesstest` (fake paperless API) and `llmtest.Fake`.

**Email order extraction:** `ledger.EmailService` (`email_service.go`, `email_pipeline.go`, `email_extract.go`, `email_preclass.go`, `email_match.go`) turns messages into `LedgerEmailOrder`s with `llm.Extractor` (injected through `ledger.Config.Extractor`, nil = disabled). One shared path (`processMessages`) serves IMAP scans and `.eml` uploads: the IMAP scan selects the account's `folder` (required; modified UTF-7 encoded), fetches only headers (`BODY.PEEK[HEADER]`) of the newest 250 messages since `max(scanSince, lastScanAt−48h)` and downloads a message only when it needs extraction. Dedup runs before any LLM call: the key is the Message-ID (`mid:<hash>` when it is longer than 200 bytes or has control characters, `nomid:<hash>` without one); a message with an order (`idx/eord_msgid`) or a processed marker (`idx/eseen`, set for `isOrder=false`, pre-classified non-orders and messages above 25 MB) is skipped. `extractOrder` sends sender, subject, date and the text body (cap 12 000 characters) with the system prompt that treats the email as untrusted data; `validateExtraction` checks total, currency, date (falls back to the Date header), payment method enum, items. Failures: `llm.ErrDisabled` → `ErrEmailLLMDisabled` before anything is downloaded (409, scheduler status "skipped"); invalid/refused/truncated output and 4xx → per-message warning, no marker, retried next scan; transient errors → warning and `skipped`, three in a row stop the scan as `partial`; a message that is unusable or fails validation is retried at every scan inside the lookback window (one LLM call each, accepted cost); IMAP I/O errors abort the scan (the connection is unusable), a tagged `NO` on a fetch only skips the message and makes the scan incomplete, but after 3 refusals of the same message (counted in `idx/erej/{accountId}/{key}`, `uid:N` for header fetches) it is marked `unreadable` and ignored; context end stops the scan and keeps stored orders. Warnings and logs contain only message labels (`uid N`, file names) and fixed texts, never email content or model output. `deriveMatchRule` (pure) gives the counterparty pattern and charge window from merchant and payment method (PayPal `paypal` −3/+7; merchant containing `amazon` without PayPal `amazon` −3/+60; otherwise the quoted merchant −3/+30; a merchant shorter than 3 characters or none → no auto-link); `matchRuleForOrder` keeps legacy importer orders unchanged. New orders have `importerId: "llm"`. Optional per-account `preClassify` first sends sender and subject of batches of 40 (fail open: anything the model does not clearly reject is extracted). One scan per account at a time; `LastScanAt` (the lookback anchor) only advances after a complete IMAP scan (no `partial`, no `skipped`); `LastScanAttemptAt` is stamped by every attempted scan and schedules the next background scan, so failed or incomplete accounts keep the normal interval while accounts skipped for a missing folder/LLM stay due each tick. Scan state is written with `SetLedgerEmailAccountScanState`, which changes only the scan fields, so edits made during a scan are kept (and account edits go through `ModifyLedgerEmailAccount`, one load-modify-store transaction); changing host, port, username, TLS, folder or the password clears `LastScanAttemptAt` so the next tick rescans; racing transactions are retried on `badger.ErrConflict`; uploads never move `LastScanAt`. Tests use `llmtest.Fake` and the fixtures in `docs/design/email/`.

**Personal access tokens:** `internal/pat` stores revocable tokens for agents and scripts. Format `kpat_` + 32 random bytes (base64url); only the SHA-256 hash is stored (`pat_hash/{hash}` → user and token ID, plus the record `u/{userId}/pat/{id}`), the raw token is returned once on creation (`Cache-Control: no-store`). Scopes are `read`, `drafts` and `links` (no write scope); at most 50 per user (409 above); optional expiry must lie in the future. `Store.Authenticate` returns `middleware.PATIdentity` or `middleware.ErrInvalidPAT` (unknown, revoked and expired alike) and updates `lastUsedAt` at most once a minute without ever resurrecting a revoked record. Tokens are outside the module prefixes and not exported. There is no account-deletion path today; a future one must delete `u/{userId}/pat/*` and the matching `pat_hash/{hash}` keys, which live outside the user prefix, so a prefix delete would leave orphaned hashes (they fail closed, but still leak hashes).

**MCP:** `POST /mcp` is a stateless streamable-HTTP MCP endpoint (`github.com/modelcontextprotocol/go-sdk` v1.8.0; `GET`/`DELETE` give 405). It sits on the outer mux next to `/api/v1/`, behind `middleware.Auth(PATOnly(), WithPATs, WithChallenge("Bearer"))` and the normal RequestID → Recovery → Metrics → Logging → CORS chain (`statusRecorder` implements `Unwrap`/`Flush` so SSE streaming works through the wrappers). Packages: `internal/mcptool` is the leaf framework (`Registrar`, `Spec{Name, Description, Scope, Module, ReadOnly}`, generic `Add[In, Out]`, `Call{UserID, TokenID, Scopes}` from the authenticated request and never from tool arguments, `ToolError` codes `insufficient_scope|module_disabled|not_found|invalid_input|already_pending|too_many_pending|paperless_not_configured|paperless_unavailable|public_url_not_configured|internal_error`, `CreateDraft`, `CheckDate`/`CheckLen`); `internal/mcpserver` mounts the handler and holds the core tools (`list_modules` read, `search_paperless_documents` read, `attach_paperless_documents` links) and the server instructions; modules implement the optional `module.ToolProvider` (`RegisterTools(*mcptool.Registrar)`), which `module.Registry.RegisterTools` calls in registration order. `Handler` builds one `mcp.Server` per request (`Registrar.Build`, shared `SchemaCache`) because the SDK asks for the server several times per stateless request. A receiving middleware (the guard) filters `tools/list` to tools whose scope the token has and whose module is enabled and answers `tools/call` of anything else with a tool error (`insufficient_scope`, `module_disabled`, `IsError`), not a protocol error. Handler errors that are not a `ToolError` (and panics) are logged with tool name and token ID and returned as the fixed `internal_error`; arguments and results are never logged. A tool failure's text is `<code>: <message>`. Draft tools never write entities: `CreateDraft` stores a pending draft with source `mcp`; an optional client `key` makes repeats `already_pending` while a draft exists (ref `k:<sha256(key)>`), without a key every call creates a new draft (ref `r:<uuid>`). Title, key and note are trimmed (a blank title or a non-empty blank key is `invalid_input`). A user can have about 500 pending MCP drafts (a soft cap: parallel calls can overshoot it by the number in flight) (`mcptool.DefaultMaxPendingDrafts`, counted in the same transaction as the create by `drafts.Store.CreateLimited`); parallel creates with the same key conflict in badger, so `CreateLimited` retries via `storage.RetryConflicts` and the loser gets `already_pending`; beyond that `CreateDraft` returns `too_many_pending` until the user decides some. Every tool sets `readOnlyHint` and `idempotentHint` and explicit `destructiveHint: false`, `openWorldHint: false` (`Spec.Idempotent` marks repeatable write tools). `attach_paperless_documents` derives the entity URL from `PUBLIC_URL` (`public_url_not_configured` when unset; there is no request-header fallback), checks that the entity exists for the user and that its module is enabled. `DisableLocalhostProtection` is on because requests carry a bearer token and a reverse proxy on the same host would otherwise be rejected. Config: `PUBLIC_URL` (absolute http(s) URL, trailing slash trimmed, no query/fragment; empty = unset). Metrics: `mcp_tool_calls_total{tool,outcome}` (`ok|tool_error|forbidden|internal_error`) and `mcp_tool_call_duration_seconds{tool}`.

**MCP tools** (scope = the one personal access token scope a tool needs; a tool of a disabled module is hidden and its call returns `module_disabled`; search tools take `limit` (default 25, max 100) and return `{items, truncated}`; every list result is an object with `items`, never a bare array; dates `YYYY-MM-DD`, amounts EUR; draft tools take `title` (required), optional `key` (idempotency) and `note`, validate with the module's input type, check that the parent exists in the right module and return `{draftId, status}`; nothing is stored when validation fails). `server/mcp_integration_test.go` pins the complete tool list per scope (`allowedTools`): a new tool must be added there deliberately.

| Tool | Scope | Module | What it does |
|------|-------|--------|--------------|
| `list_modules` | read | core | Modules and their enabled state |
| `search_paperless_documents` | read | core | Paperless full-text search (`query`, `page`) |
| `attach_paperless_documents` | links | core | Link paperless documents to an existing contract, purchase, vehicle, cost or transaction (needs `PUBLIC_URL`) |
| `list_contract_categories` | read | contracts | Category IDs and names |
| `search_contracts` | read | contracts | `query` (name, product, company, contract number), `categoryId` |
| `create_contract_draft` | drafts | contracts | `categoryId` + `ContractInput`; target `contract` |
| `create_contract_category_draft` | drafts | contracts | `{name}`; target `contract_category` |
| `list_purchase_categories` | read | purchases | Category IDs and names |
| `search_purchases` | read | purchases | `query` (item, brand, dealer, article number), `categoryId`, `dateFrom`/`dateTo` |
| `create_purchase_draft` | drafts | purchases | `categoryId` + `PurchaseInput`; target `purchase` |
| `create_purchase_category_draft` | drafts | purchases | `{name}`; target `purchase_category` |
| `list_vehicles` | read | auto | Vehicle IDs, name, make, model, year, plate |
| `search_vehicle_costs` | read | auto | `vehicleId`, `query` (description, vendor), `type`, `dateFrom`/`dateTo` |
| `create_vehicle_cost_draft` | drafts | auto | `vehicleId` + `CostEntryInput`; target `cost` |
| `create_vehicle_draft` | drafts | auto | `VehicleInput`; target `vehicle` |
| `list_ledger_accounts` | read | ledger | Accounts with `ibanLast4` only (never the full IBAN) |
| `list_ledger_categories` | read | ledger | Category IDs, names, parent IDs |
| `search_ledger_transactions` | read | ledger | `query` (counterparty, purpose, note), `dateFrom`/`dateTo` (booking date), `minAmount`/`maxAmount` (absolute, major units, 0 to 10^12), `accountId`; results carry the signed `amount`, `purpose` cut to 200 characters, references, no counterparty IBAN |
| `create_ledger_category_draft` | drafts | ledger | `{name, matchWords}` plus optional `parentId` (goes to the draft's `parentId`, not into `input`); target `ledger_category` |
| `create_ledger_account_draft` | drafts | ledger | `{name, bank, iban?, currency?}` (`ValidateForCreate`; an IBAN that already exists is rejected); target `ledger_account` |
| `link_transaction` | links | ledger | Add one reference (`targetType` contract, purchase or vehicle) through the same code as `POST /ledger/transactions/{id}/references`; `{linked, alreadyLinked}` (decided inside the write transaction, exact under concurrency; the tool is annotated idempotent); `module_disabled` when the target's module is disabled |

That is 21 tools in total: 11 read, 8 drafts and 2 links (`allowedTools` in the integration test is the authoritative list). Ledger transaction search reads all of the user's transactions and filters in memory, like the list endpoints.

**Logging:** `log/slog` — JSON handler in production, text in dev.

**Metrics:** Prometheus via `/metrics` endpoint. Tracks `mcp_tool_calls_total` (see MCP), `http_requests_total`, `http_request_duration_seconds`, `http_active_requests`, and for the LLM client `llm_requests_total{operation,outcome}`, `llm_request_duration_seconds{operation}`, `llm_tokens_total{operation,type}`, `llm_retries_total{operation}`.

**Middleware chain (outermost first):** RequestID → Recovery → Metrics → Logging → CORS → handler. Module routes additionally pass the enablement gate after auth.

**Auth:** JWT-based authentication. Registration and login seed default data for every enabled module (idempotent; re-enabling a module in settings re-seeds it). Stores accept `userID` as a parameter; handlers extract it from auth context. Passwords must be at least 8 characters (register and change). `/auth/login` and `/auth/register` share a per-IP token-bucket rate limit (`internal/middleware/ratelimit.go`): `AUTH_RATE_LIMIT` requests (default 10) per `AUTH_RATE_WINDOW` (default `1m`), 0 disables; set `TRUST_PROXY=true` behind a reverse proxy so client IPs come from `X-Real-IP`/`X-Forwarded-For`.

**Personal access tokens in the API:** `middleware.Auth(secret, opts...)` accepts a JWT or a personal access token (`kpat_…`, `WithPATs(verifier)`); it puts a `middleware.Principal` (`UserID`, `Kind` `jwt|pat`, `TokenID`, `Scopes`) into the context (`GetPrincipal`, `IsPAT`, `HasScope`; `GetUserID` keeps working). Failures are an identical `401 {"error":"unauthorized"}` for unknown, revoked, expired and malformed tokens (a verifier error is `500`); options `PATOnly()` (reject JWTs) and `WithChallenge("Bearer")` (set `WWW-Authenticate`) are meant for `/mcp`. Tokens are never logged. Reach is an explicit table keyed by the registered mux pattern (`internal/server/scopes.go`, enforced by `middleware.ScopeGuard` around the API mux, JWT requests pass untouched, a pattern missing from the table is denied): `read` for every `GET` (including the exports), `drafts` for `PUT /drafts/{id}` and the draft accept/reject, `links` for `POST /paperless/links/{entityType}/{entityId}` and `POST /ledger/transactions/{id}/references`, `denied` (never reachable with a token) for every other mutation, `jwt-only` for `/tokens*` and `PUT /settings/password` (`/auth/*` is public and outside the API mux). A rejected request gets `403 {"error":"insufficient scope","code":"insufficient_scope"}`; 404/405 are answered by the mux as usual and module gating still applies afterwards. A new route must get a table entry: `TestRouteRulesCoverRoutes` builds the real app (`buildApp`) and fails for missing or stale entries. **Accepted residual risk (decided with the user, not changed):** over the REST API the scopes are broader than the MCP tools. A `drafts` token can accept drafts, including with an unverified `entityId` (accept does not check that the entity exists), so an agent can clear its own drafts from the inbox without the user reviewing them; for **paperless-sourced** drafts, accepting also stores a paperless link for the unchecked entity, writes the request's `entityUrl` into the document's "Kontor" custom field and swaps the inbox tag for the done tag in paperless, and rejecting removes the inbox tag; `PUT /drafts/{id}` can edit the input of any pending draft; and a `links` token chooses the `entityUrl` that is written into the user's paperless "Kontor" custom field and skips the entity-exists check of `POST /paperless/links/{entityType}/{entityId}`. The MCP tools do not have these gaps: `attach_paperless_documents` derives the URL from `PUBLIC_URL` and verifies the entity, and there is no MCP tool to accept drafts. Give tokens only the scopes a client needs, and REST access only to clients you trust with these.

## Key directories

- `cmd/server/` — Entrypoint (opens the engine, starts the server)
- `internal/config/` — Environment-based configuration
- `internal/storage/` — Badger engine, JSON txn helpers, sentinel errors, backups; `migration/` (per-module runner), `link/` (cross-module link registry)
- `internal/module/` — Module interface, registry, gating router/middleware, import result
- `internal/core/` — Users, settings (incl. enabled modules), auth/settings/health/capabilities handlers, export/import orchestrator
- `internal/categories/` — Shared item-category machinery for contracts and purchases (model, store with pluggable delete cascades, handlers)
- `internal/modules/contracts/` — Contracts module incl. renewal reminder scheduler and batch item import
- `internal/modules/purchases/` — Purchases module
- `internal/modules/auto/` — Vehicles and cost entries
- `internal/modules/ledger/` — Ledger module: accounts, hierarchical categories, transactions, CSV import (`import_*.go`, comdirect/DKB providers), email-order enrichment via LLM extraction (`email_*.go`, IMAP scan scheduler via `LEDGER_EMAIL_SCAN_INTERVAL`, default `6h`, `0` disables), keyword categorization
- `internal/httputil/` — Shared JSON response helpers
- `internal/middleware/` — Request ID, recovery, metrics, logging, CORS, auth, rate limiting
- `internal/email/` — SMTP client
- `internal/cryptoutil/` — AES-GCM string encryption (email passwords, paperless tokens), keyed by `EMAIL_ENCRYPTION_KEY`
- `internal/paperless/` — Optional paperless-ngx integration: per-user config (encrypted API token) and pipeline settings, outbound HTTP client (search, thumbnails, custom-field back-links, tags, document content, bulk tag edits), central document-link store, `Service` (shared client resolution, attach/detach with done tag), handlers, the draft pipeline (`pipeline.go`) and its draft hooks; `paperlesstest/` holds a fake paperless API for tests. Routes are ungated (not module-scoped); handlers return 409 when the user has not configured the integration. Deleting an entity leaves its paperless links as unreachable keys (accepted; no sweep yet)
- `internal/mcptool/` — MCP tool framework (registrar, per-request server with scope/module guard, error mapping, metrics, draft helper); leaf package imported by modules
- `internal/mcptool/mcptooltest/` — Test helper: registrar plus in-memory MCP client for module tool tests
- `internal/mcpserver/` — `/mcp` handler (stateless streamable HTTP) and the core tools
- `internal/pat/` — Personal access tokens: model, hash-only store, token handlers
- `internal/drafts/` — Source-agnostic draft inbox (model, store, handlers, source hook registry); routes are ungated (not module-scoped), entities are created through the gated module routes
- `internal/extraction/` — Shared LLM classifier (purchase / contract / cost / none) with output validation and EUR conversion; used by the paperless pipeline and quick add
- `internal/fx/` — ECB exchange rates via a Frankfurter-compatible API (`FX_RATES_URL`)
- `internal/quickadd/` — `POST /api/v1/quick-add` handler (typed note → proposal, nothing stored)
- `internal/llm/` — Optional server-wide LLM client for structured extraction (config-driven `Extractor`, strict-mode schema generation, metrics); `llmtest/` holds a scripted `Fake` for tests
- `internal/server/` — Module wiring (`buildApp`: handler plus recorded route patterns, `start` launches background services), the PAT scope table (`scopes.go`), mux setup, middleware, graceful shutdown, SPA serving
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
- `POST /api/v1/ledger/accounts` — Create a ledger account (`{name, bank, iban?, currency?}`; `ValidateForCreate`: trimmed, currency upper-cased three letters (default `EUR`), IBAN normalized and mod-97 checked; 409 for a duplicate IBAN; gated on the ledger module, not reachable with a personal access token)
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
- `POST /api/v1/ledger/transactions/{transactionId}/references` — Add one reference (`{type: purchase|contract|vehicle, targetId}`) and the target's back-link; idempotent, removes nothing, leaves category, transfer state, note and links alone; 404 for an unknown transaction or target, 403 `module_disabled` when the target's module is disabled
- `GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates` — List internal transfer candidates
- `POST /api/v1/ledger/transactions/{transactionId}/transfer-link` — Link internal transfer pair
- `DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link` — Unlink internal transfer pair
- `POST /api/v1/ledger/transactions/{transactionId}/review` — Review/categorize transaction
- `GET|POST /api/v1/ledger/email-accounts` — List/create ledger email accounts
- `GET|PUT|DELETE /api/v1/ledger/email-accounts/{emailAccountId}` — Get/update/delete ledger email account
- `POST /api/v1/ledger/email-accounts/{emailAccountId}/test` — Test the configured IMAP connection using stored credentials
- `POST /api/v1/ledger/email-accounts/{emailAccountId}/scan` — Scan the account's IMAP folder with the LLM and auto-link the extracted orders; multipart `.eml` upload (field `files`) processes uploaded messages the same way. 409 when the LLM is not configured, the account has no folder (IMAP scan) or a scan of the account is already running. A scan can take minutes (sequential LLM calls) and is not cancelled when the client disconnects; it is bounded by a 10 minute timeout: on timeout, repeated LLM failures or skipped messages the result carries `partial: true` / `skipped: N` and the orders created so far (HTTP 200)
- `GET /api/v1/ledger/email-orders` — List parsed ledger email orders
- `GET /api/v1/ledger/email-orders/{emailOrderId}` — Get parsed ledger email order
- `GET /api/v1/ledger/email-orders/{emailOrderId}/candidates` — List unlinked outgoing transactions from all accounts within the charge window around the order date, derived from the order's merchant and payment method (PayPal −3/+7 days, Amazon not paid via PayPal −3/+60, anything else −3/+30; orders stored before the LLM extraction keep the window of their `amazon.de`/`paypal.de` importer); `?afterDays=1..365` widens the window
- `POST /api/v1/ledger/email-orders/{emailOrderId}/link` — Manually link parsed email order to ledger transactions
- `POST /api/v1/ledger/email-orders/{emailOrderId}/reject` — Reject parsed email order
- `GET|POST /api/v1/tokens` — List/create personal access tokens (`{name, scopes, expiresAt?}`; the raw `token` is only in the create response)
- `DELETE /api/v1/tokens/{id}` — Revoke a personal access token
- `GET /api/v1/export` — Download all user data (v2 envelope with per-module sections)
- `GET /api/v1/modules/{module}/export` — Download a single module's data
- `POST /api/v1/import` — Import an export file (affected modules must be empty and enabled; preserves IDs; email passwords must be re-entered)
- `POST /api/v1/modules/{module}/import` — Import a single module's section from an export file
- `GET /api/v1/modules` — List available modules and their enabled state
- `GET /api/v1/capabilities` — Server-wide feature flags (`{"llm": bool}`; `llm` is true when `LLM_BASE_URL` is configured)
- `GET /api/v1/settings` — Get settings (renewal preferences, enabled modules)
- `PUT /api/v1/settings` — Update settings (`enabledModules` optional; omit to leave unchanged)
- `PUT /api/v1/settings/password` — Change password
- `GET /api/v1/paperless/config` — Paperless integration status (`{configured, baseUrl}`; the token is never returned)
- `PUT /api/v1/paperless/config` — Save paperless base URL + API token (token optional on update; encrypted with `EMAIL_ENCRYPTION_KEY`)
- `DELETE /api/v1/paperless/config` — Remove the paperless connection (document links are kept)
- `POST /api/v1/paperless/config/test` — Test the stored paperless connection (502 on upstream failure)
- `GET /api/v1/paperless/search?query=&page=` — Proxy full-text document search (empty query lists recent documents)
- `GET /api/v1/paperless/documents/{documentId}/thumb` — Proxy document thumbnail (authenticated stream)
- `GET /api/v1/paperless/links/{entityType}/{entityId}` — List paperless document links for an entity
- `POST /api/v1/paperless/links/{entityType}/{entityId}` — Attach documents (`{entityUrl, documents}`); best-effort sets a "Kontor" URL custom field and the done tag on each paperless document, failures come back as warnings
- `DELETE /api/v1/paperless/links/{entityType}/{entityId}/{documentId}` — Detach a document; clears the back-link custom field only if it still points at this entity, and removes the done tag when no other entity links the document
- `GET /api/v1/paperless/pipeline` — Draft pipeline settings (`{enabled, inboxTag, doneTag}`; defaults `false`, `kontor-inbox`, `kontor`)
- `PUT /api/v1/paperless/pipeline` — Save pipeline settings (tags non-empty, at most 128 characters, distinct case-insensitively)
- `POST /api/v1/paperless/pipeline/run` — Run a pipeline pass for the current user now (`{found, created, skipped, failed}`; 409 when the LLM is off, paperless is not configured, the pipeline is disabled, no target module is enabled or a pass is already running; 502 on paperless errors)
- `GET /api/v1/drafts?status=` — List drafts, newest first (`pending` by default; `accepted`, `rejected`, `all`)
- `GET /api/v1/drafts/summary` — `{pending, total}` draft counts
- `GET /api/v1/drafts/{id}` — Get a draft
- `PUT /api/v1/drafts/{id}` — Edit a pending draft (`{targetType, parentId, input}`; 400 for an unknown type or a parent on a type without parents; 409 when decided)
- `POST /api/v1/drafts/{id}/accept` — Accept a pending draft after the frontend created the entity (`{entityType, entityId, entityUrl}`; `entityType` must match `targetType`); runs the source hook (paperless: link, back-link, tag swap) and returns `{draft, warnings}`; 409 when decided
- `POST /api/v1/drafts/{id}/reject` — Reject a pending draft; runs the source hook (paperless: remove inbox tag) and returns `{draft, warnings}`; 409 when decided
- `POST /api/v1/quick-add` — Classify a typed note (`{text, targetType?}`, at most 2000 characters; `targetType` forces `purchase|contract|cost`) into `{targetType, parentId, input, confident, note, currency?}` (`currency{code, amount, converted, rate?, rateDate?}` for non-EUR amounts); nothing is stored. 409 when the LLM is off or no target module is enabled, 400 for empty/too long text or an invalid/disabled `targetType`, 504 on LLM timeout, 502 on other transient LLM errors; non-retryable LLM errors return `targetType: none` with a note
- `POST /mcp` — MCP endpoint for agents (personal access token only; `initialize`, `tools/list`, `tools/call`; 401 + `WWW-Authenticate: Bearer` otherwise)
- `GET /healthz` — Liveness probe
- `GET /readyz` — Readiness probe (checks DB)
- `GET /metrics` — Prometheus metrics

Linked internal transfers are intentionally protected from normal category assignment until the user explicitly unlinks them.
