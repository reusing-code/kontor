# Kontor

A self-hosted personal finance manager for tracking contracts, subscriptions, purchases, vehicle costs, and ledger transactions. Built with a Go backend and React frontend.

## Features

- **Modular design** — First-class modules (contracts, purchases, auto, ledger) with independent dashboards; enable or disable modules per account (data is kept while disabled)
- **Contract tracking** — Store contract details including dates, pricing, notice periods, and renewal terms
- **Purchase tracking** — Track one-time purchases with item details, pricing, dealer info, and document links
- **Vehicle cost tracking** — Manage vehicles with cost entries (service, fuel, insurance, tax, tires, mileage, misc), a per-vehicle statistics page with charts, and total cost of ownership projections
- **Ledger module** — Import bank CSVs into tracked accounts, review transactions, manage categories, add notes/links/references, mark internal transfers, and enrich transactions with email order data extracted by the LLM
- **Category suggestions (needs the LLM)** — Unreviewed transactions that no match-word rule catches get LLM category suggestions with a confidence, shown preselected in the review queue: accept them all at once or one by one, and optionally turn a proposed match word into a rule. Rules always win (a run re-applies them first). Runs in the background after every import and on demand ("Suggest categories"); per transaction only booking date, amount, counterparty and purpose are sent, plus the category tree and a few already categorized transactions with similar counterparties. Batch size and per-run cap: `LEDGER_SUGGEST_BATCH_SIZE` (default 25), `LEDGER_SUGGEST_MAX_PER_RUN` (default 100)
- **Recurring payments as contracts** — After every import (and on demand) Kontor looks for recurring outgoing EUR payments: same counterparty, an amount that stays within 10 % (at least €1) of the previous one, and a monthly, quarterly or yearly rhythm. Each group is proposed as a contract draft in the inbox (`/inbox`), also when the payments have stopped (then with an end date); accepting links all its payments to the new contract, a rejected group is not proposed again. Pure code, no LLM
- **Email order enrichment (needs the LLM)** — Configure IMAP email accounts with the folder (or Gmail label) that holds your order emails; the LLM reads each new message of that folder (any merchant, any language), extracts merchant, payment method, order ID, date, total and items, and Kontor auto-links the order to the matching ledger transaction. An order in another currency than the paying account (e.g. a USD PayPal payment) keeps its original amount; for matching it is converted at the ECB rate of the order date and linked when exactly one payment lies within ±5 % of that (conversion fees), and the review dialog shows the converted amount, or a hint that amounts are not comparable when no rate is available. Only sender, subject, date and the text body are sent to the LLM, and only for messages in the chosen folder. An optional per-account pre-selection by sender and subject skips obvious non-orders with one cheap call per batch. Messages are processed once (known orders and non-orders are not sent again); a message the LLM cannot extract is retried on later scans and ignored after 3 unusable answers (`LEDGER_EMAIL_MAX_EXTRACTION_FAILURES`). Accounts are also scanned automatically in the background (`LEDGER_EMAIL_SCAN_INTERVAL`, default every 6h); `.eml` upload is available to process old messages. Without the LLM nothing is scanned (existing orders, linking and candidates keep working); existing accounts need a folder chosen before they are scanned
- **Paperless-ngx integration** — Optionally connect a paperless-ngx instance per user (base URL + API token in settings); search the document archive and attach documents to contracts, purchases, vehicles, cost entries, and ledger transactions, with an automatic back-link ("Kontor" URL custom field) on the paperless document
- **Draft inbox from paperless (optional)** — With the LLM endpoint and paperless configured, tag a document with `kontor-inbox` in paperless and Kontor proposes a purchase, contract or vehicle cost from its text in the inbox (`/inbox`); accepting opens the prefilled create dialog, links the document and replaces the tag with `kontor`. The inbox backend also holds drafts for vehicles, contract/purchase/ledger categories and ledger accounts (created by agents, see the personal access tokens). Enabled per user in settings (tag names configurable); polled every `PAPERLESS_PIPELINE_INTERVAL` (default `15m`, `0` disables)
- **Quick add (optional)** — With the LLM endpoint configured, the header's quick-add button (or Ctrl/Cmd+K) turns a typed note such as "bought winter tires 2026-11-02 at Reifen Müller, €640, Golf" into the prefilled create dialog of a purchase, contract or vehicle cost; unclear notes ask for the type and category/vehicle first. Instead of typing you can attach an invoice or receipt (at most 10 MB), with an optional note as a hint: a PDF is read from its text layer (a scan without text is refused, there is no OCR), a photo (JPEG, PNG, WebP; file picker or camera on a phone) is scaled down in the browser and sent to the vision model, which needs `LLM_VISION_MODEL`. Nothing is stored until you save the dialog, and the file is not kept
- **MCP endpoint for agents** — `POST /mcp` (stateless streamable HTTP, official Go SDK) lets agents such as Claude Code or OpenCode work with your data using a personal access token (`kpat_…`, PAT only, no JWT). Agents can only read reference data, search paperless and create drafts that you review in the inbox (and link paperless documents); they cannot create or change entities. Tools are filtered by the token's scopes and by your enabled modules. Set `PUBLIC_URL` (e.g. `https://kontor.example.com`) so the back-link Kontor writes into paperless points at the right address; without it `attach_paperless_documents` reports `public_url_not_configured`
- **Foreign currencies** — Kontor stores amounts in EUR. When the LLM finds another currency (quick add, paperless drafts), the amount is converted at the ECB reference rate of the entry's date via the Frankfurter API (`FX_RATES_URL`, default `https://api.frankfurter.dev/v1`, `off` disables) and the original amount is kept in the comments; without a rate the amount is left empty for you to fill in. Email orders are the exception: they keep the extracted currency and are only converted for matching (see above)
- **LLM endpoint (optional)** — Server-wide OpenAI-compatible endpoint for structured extraction (`LLM_BASE_URL`, `LLM_MODEL`, optional `LLM_API_KEY`, `LLM_TIMEOUT` default `60s`; optional `LLM_VISION_MODEL`, a vision-capable model on the same endpoint that receives the photos attached in quick add, without it only PDFs can be attached); off while `LLM_BASE_URL` is unset, and the UI only offers LLM features when `/api/v1/capabilities` reports them. Used by quick add, the paperless pipeline, ledger email order extraction and ledger category suggestions. Prompts and model output are never logged
- **Automatic bank import (optional)** — Connect DKB, comdirect or any bank that Enable Banking lists (`GET /ledger/bank/aspsps`; DKB and comdirect are the ones this was built for and are marked "verified") and Kontor imports new transactions in the background (`ENABLE_BANKING_SYNC_INTERVAL`, default `24h`, `0` = manual "Sync now" only). Imports use the same keyword categories and review queue as CSV imports; transactions that an earlier CSV import already holds are recognized and not duplicated. Consents last at most 180 days; Kontor warns in the UI and, with SMTP configured, by mail 14 and 3 days before the end. See "Bank import (Enable Banking)" below
- **Category organization** — Per-module categories (e.g. insurance/telecom for contracts, PC hardware/tools for purchases)
- **Homepage overview** — Dashboard at `/` with summary cards and stats across all modules
- **Renewal monitoring** — Upcoming renewals with color-coded urgency indicators
- **Email reminders** — Configurable SMTP-based reminder emails for approaching renewals
- **Batch import** — Import contracts from JSON via file upload or paste
- **Multi-user** — JWT authentication with per-user data isolation
- **Data export & import** — Download all data or a single module as JSON (versioned envelope) and import it back per module or all at once; optional periodic BadgerDB snapshots with retention (`BACKUP_DIR`, `BACKUP_INTERVAL`, `BACKUP_KEEP`)
- **Observability** — Prometheus metrics, structured logging, health/readiness probes

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Frontend | React 19, TypeScript, Vite, TanStack Router/Query, shadcn/ui, Tailwind CSS |
| Backend | Go (stdlib `net/http`), BadgerDB, JWT, slog |
| Runtime | Docker, Alpine Linux |

## Quick Start

### Prerequisites

- [Go 1.27+](https://go.dev/dl/) (pinned in `backend/go.mod`)
- [Bun 1.4.4](https://bun.sh) (pinned in `frontend/package.json` `engines.bun`)
- [golangci-lint](https://golangci-lint.run) (optional; pinned in `.tool-versions`)
- [Task](https://taskfile.dev) — `go install github.com/go-task/task/v3/cmd/task@latest`
- [Air](https://github.com/air-verse/air) — `go install github.com/air-verse/air@latest`

### Development

```sh
task dev
```

This starts the Go backend (port 8080) with live reload and the Vite dev server (port 5173) with API proxying. Open <http://localhost:5173>.

### Docker

```sh
task build   # build image
task up       # start with docker compose
task down     # stop
```

`task build` passes the Bun version from `frontend/package.json` and the Go version from `backend/go.mod` as build args; when running `docker build` directly, add `--build-arg BUN_VERSION=<version> --build-arg GO_VERSION=<version>`.

CI publishes the image to `forgejo.reusing.dev/tobi/kontor` and mirrors it to `ghcr.io/reusing-code/kontor`, which `compose.yml` uses. Tags: `sha-<commit>`, `main`, and per release `X.Y.Z`, `X.Y` and `latest`.

Every push to `main` adds a `sha-*` tag on Forgejo. A package cleanup rule set in the Forgejo UI (user settings → Packages → Cleanup rules → Add cleanup rule) keeps that bounded while keeping release tags, `main` and `latest`:

- Type: Container, "Apply pattern to full package name" checked
- Keep the most recent: 10 versions per package
- Remove versions matching: `^kontor/sha-`

The remove pattern limits the rule to the `sha-*` tags of `kontor` (not `kontor-ci`); the preview in the rule editor shows what would be deleted.

## Bank import (Enable Banking)

The bank import is off until you configure it; the UI shows nothing about it while `/api/v1/capabilities` reports `bankImport: false`.

**Privacy.** Bank data passes through [Enable Banking](https://enablebanking.com) (an account-information provider licensed under PSD2); Kontor has no other path to your bank (FinTS is not used). What Kontor sends to Enable Banking: the bank you pick, the consent validity and scope (read access to accounts and transactions, with extended history), the redirect URL, the bank's own account UIDs and date ranges when it fetches transactions. It never sends your ledger data, categories or notes. What Kontor stores from the bank: an encrypted session ID (AES-GCM with `ENCRYPTION_KEY`; it is never returned by the API or written to logs), the consent's validity, the account list (UID, IBAN, name, currency) and the transactions the bank returns, in the same form as CSV-imported ones. Consents, mappings and sync state are not part of the export; after restoring a backup, connect the banks again (re-authorize). The integration is operator-only: a restricted-mode Enable Banking app can only access its owner's accounts, which is the expected setup for a personal install.

**Setup.**

1. Register an application at Enable Banking, generate an RSA key pair, upload the certificate and keep the private key (PEM). Link your own bank accounts to the application (restricted mode).
2. Register the redirect URL **exactly** `PUBLIC_URL/api/v1/ledger/bank/callback` in the application (for example `https://kontor.example.com/api/v1/ledger/bank/callback`).
3. Set `ENABLE_BANKING_APP_ID`, `ENABLE_BANKING_KEY_PATH` (mount the key file read-only into the container, see `compose.yml`), `PUBLIC_URL` and `ENCRYPTION_KEY`. Optional: `ENABLE_BANKING_SYNC_INTERVAL` (default `24h`; `0` disables only the automatic sync, expiry mails keep working), `ENABLE_BANKING_API_URL` (default `https://api.enablebanking.com`). Configure SMTP for the expiry mails.
4. Open Ledger → Bank connections, connect a bank, approve at the bank, then map each bank account to a ledger account (matched by IBAN automatically; or create the ledger account from the bank account).

**Two residual risks of the public callback.** The callback is public by design, so its `state` is not bound to the browser that started the consent: if someone gets you to finish a consent that was started from *their* Kontor session, your bank session would be attached to their user. Restricted mode and a single-owner install make this very unlikely, but do not open bank-consent links you did not start yourself. And `code` and `state` are part of the callback URL, so a reverse proxy's access log will contain them; both are single use and short lived (the state lives 15 minutes), but do not keep those access logs longer than needed.

Start with a CSV import for the history you already have and connect the bank afterwards; a CSV imported over API-synced history is not matched against it. How CSV and API data differ and how duplicates are avoided is described in `docs/design/enablebanking/README.md`.

**Development.** The consent redirects the browser to `PUBLIC_URL`, so for local development set `PUBLIC_URL=http://localhost:5173` (the Vite dev server proxies `/api` to the backend) and register that origin's callback URL in the Enable Banking application. Tests use an in-memory fake API (`internal/enablebanking/enablebankingtest`) and never contact Enable Banking.

## Using Kontor from an agent (MCP)

Create a personal access token in **Settings → Access tokens** (shown once; optional expiry, 90 days by default), then point an MCP client at `https://<your-kontor>/mcp` with the header `Authorization: Bearer <token>`:

```bash
# Claude Code
claude mcp add --transport http kontor https://kontor.example.com/mcp --header "Authorization: Bearer <your-token>"
```

```json
// opencode.json
{ "mcp": { "kontor": { "type": "remote", "url": "https://kontor.example.com/mcp", "headers": { "Authorization": "Bearer <your-token>" } } } }
```

| Scope | Allows |
|-------|--------|
| `read` | Every read: list and search tools in MCP, and every `GET` of the REST API including the full data export |
| `drafts` | Create drafts through MCP (contracts, purchases, vehicle costs, vehicles, categories, ledger accounts) and edit/accept/reject drafts through the REST API. Over REST a `drafts` token can accept drafts itself, even with an entity ID that was never created, so it can clear its own drafts from the inbox without your review; for drafts that came from paperless, accepting also stores a link for that unchecked item, writes the URL from the request as back-link and swaps the inbox tag for the done tag in paperless (rejecting removes the inbox tag), and it can edit any pending draft |
| `links` | Link paperless documents to existing items and add references from ledger transactions to contracts, purchases, vehicles and vehicle cost entries. Over REST a `links` token chooses the back-link URL that is written into paperless and skips the check that the item exists (the MCP tool derives the URL from `PUBLIC_URL` and verifies the item) |

Agents never create or change items directly through MCP: they create drafts that you review in the inbox (the broader REST scopes are described above and in the security note). Tools of modules you disabled are hidden. Set `PUBLIC_URL` on the server so back-links written into paperless point at your Kontor.

**Security:** a token with `read` can read all of your data, including exports, so treat tokens like passwords: give each client its own token with the fewest scopes and an expiry, and revoke a leaked token immediately (Settings → Access tokens). Only a hash of each token is stored; creating and revoking tokens and changing the password always need your login. Known and accepted: through the REST API a `drafts` token can accept drafts and a `links` token controls the paperless back-link URL, so only give REST access to clients you trust with those scopes (for paperless-sourced drafts `drafts` also changes the back-link and tags in paperless).

## Project Structure

```
frontend/          React SPA (Vite, TanStack, shadcn/ui)
backend/           Go API server (net/http, BadgerDB)
Dockerfile         Multi-stage build (Bun → Go → Alpine)
compose.yml        Single-service deployment with named volume
ci/                CI image and helper scripts (see AGENTS.md, "CI")
.tool-versions     golangci-lint version pin
```

See `frontend/AGENTS.md` and `backend/AGENTS.md` for per-project details.

## API

All endpoints under `/api/v1/`. Auth endpoints are public; everything else requires a bearer token: the JWT from login, or a personal access token (`kpat_…`, created through `/tokens`) within its scopes. `read` allows every `GET` (including the full export, so it exposes all your data), `drafts` the draft decisions (`PUT /drafts/{id}`, accept, reject) and `links` attaching paperless documents and adding transaction references; every other write, token management and the password change need the JWT. A request outside the token's scopes gets `403 {"code":"insufficient_scope"}`. Note that over REST a `drafts` token can accept drafts (even with an entity ID that was never created; for paperless-sourced drafts that also stores a link, writes the request's URL as back-link and changes the inbox/done tags in paperless, and rejecting removes the inbox tag) and edit any pending draft, and a `links` token supplies the back-link URL written into paperless and skips the entity check; the MCP tools are stricter (they cannot accept drafts and derive the URL from `PUBLIC_URL`).

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/register` | Register user |
| POST | `/auth/login` | Login (returns JWT) |
| GET | `/ledger/bank/aspsps` | Banks Enable Banking offers for `?country=DE` (JWT only) |
| GET | `/ledger/bank/connections` | Bank connections with account mapping and sync status (never the session) |
| POST | `/ledger/bank/connections/authorize` | Start a bank consent (`{aspspName, aspspCountry}` → `{url}`) |
| DELETE | `/ledger/bank/connections/{id}` | Disconnect a bank (`{remoteClosed}`) |
| PUT | `/ledger/bank/connections/{id}/accounts/{uid}` | Map a bank account to a ledger account (`{ledgerAccountId}`) |
| POST | `/ledger/bank/connections/{id}/sync` | Sync a connection now (409 `sync_running` / `consent_expired`) |
| GET | `/ledger/bank/callback` | Public redirect target of the consent (state is the credential; answers `303` to the result page) |
| GET | `/ledger/accounts` | List tracked ledger accounts |
| POST | `/ledger/accounts` | Create a ledger account (`{name, bank, iban?, currency?}`, IBAN checksum validated, duplicate IBAN 409; JWT only) |
| GET | `/ledger/accounts/{id}` | Get ledger account details |
| GET | `/ledger/accounts/{id}/transactions` | List transactions for an account |
| GET | `/ledger/transactions` | Review queue for ledger transactions |
| GET | `/ledger/transactions/{id}` | Get ledger transaction details |
| PUT | `/ledger/transactions/{id}` | Update note, links, and cross references |
| POST | `/ledger/transactions/{id}/references` | Add one cross reference (`{type, targetId}`, type `purchase`, `contract`, `vehicle` or `cost`; idempotent, never removes anything; `links` scope) |
| GET/POST | `/tokens` | List / create personal access tokens (JWT only; the raw token is returned once) |
| DELETE | `/tokens/{id}` | Revoke a personal access token (JWT only) |
| GET | `/ledger/transactions/{id}/transfer-candidates` | List matching internal transfer candidates |
| POST | `/ledger/transactions/{id}/transfer-link` | Link two transactions as an internal transfer |
| DELETE | `/ledger/transactions/{id}/transfer-link` | Explicitly unlink an internal transfer |
| POST | `/ledger/transactions/{id}/review` | Confirm or categorize a ledger transaction |
| POST | `/ledger/transactions/review-batch` | Confirm up to 100 transactions with a category each (accept suggestions); per-item failures |
| POST | `/ledger/category-suggestions/run` | Ask the LLM for category suggestions now (409 without LLM or while a run is going) |
| GET | `/ledger/category-suggestions/status` | `{running}` for the current user |
| POST | `/ledger/recurring/run` | Detect recurring payments now and propose new groups as contract drafts (`{groups, created, skipped}`; 409 while a run is going, 403 `module_disabled` when contracts are disabled) |
| GET/POST | `/ledger/categories` | List / create ledger categories |
| GET/PUT/DELETE | `/ledger/categories/{id}` | Ledger category CRUD |
| GET | `/ledger/imports` | List ledger import batches |
| POST | `/ledger/imports/preview` | Preview a ledger import file |
| POST | `/ledger/imports/{previewId}/commit` | Commit a previewed ledger import |
| GET/POST | `/modules/{module}/categories` | List / create categories (module: `contracts` or `purchases`) |
| GET/PUT/DELETE | `/modules/{module}/categories/{id}` | Category CRUD (cascade deletes items) |
| GET/POST | `/categories/{id}/contracts` | Contracts in category |
| GET | `/contracts` | List all contracts |
| GET/PUT/DELETE | `/contracts/{id}` | Contract CRUD |
| GET | `/contracts/upcoming-renewals` | Renewals by date |
| POST | `/contracts/import` | Batch JSON import |
| GET/POST | `/categories/{id}/purchases` | Purchases in category |
| GET | `/purchases` | List all purchases |
| GET/PUT/DELETE | `/purchases/{id}` | Purchase CRUD |
| GET | `/purchases/summary` | Purchase spending stats |
| GET/POST | `/vehicles` | List / create vehicles |
| GET/PUT/DELETE | `/vehicles/{id}` | Vehicle CRUD |
| GET | `/vehicles/{id}/summary` | Vehicle cost summary + projection |
| GET/POST | `/vehicles/{id}/costs` | List / create cost entries |
| GET/PUT/DELETE | `/costs/{id}` | Cost entry CRUD |
| GET/POST | `/ledger/email-accounts` | List / create ledger email accounts |
| GET/PUT/DELETE | `/ledger/email-accounts/{emailAccountId}` | Ledger email account CRUD |
| POST | `/ledger/email-accounts/{emailAccountId}/test` | Test the configured IMAP connection |
| POST | `/ledger/email-accounts/{emailAccountId}/scan` | Scan the account's folder via IMAP (or process uploaded `.eml` files) with the LLM and match resulting orders to ledger transactions; 409 when the LLM is off, no folder is set or a scan is already running |
| GET | `/ledger/email-orders` | List extracted email orders |
| GET | `/ledger/email-orders/{emailOrderId}` | Get extracted email order details |
| GET | `/ledger/email-orders/{emailOrderId}/candidates` | List unlinked outgoing transactions within the charge window derived from the order's merchant and payment method (`?afterDays=` widens it); for orders in another currency with `conversions` (the total per account currency at the ECB rate) or `unconvertedCurrencies`, and `amountComparable` per item |
| POST | `/ledger/email-orders/{emailOrderId}/link` | Manually link an email order to one or more ledger transactions |
| POST | `/ledger/email-orders/{emailOrderId}/reject` | Reject an extracted email order |
| GET | `/export` | Download all user data (v2 envelope) |
| GET | `/modules/{module}/export` | Download a single module's data |
| POST | `/import` | Import an export into empty, enabled modules |
| POST | `/modules/{module}/import` | Import a single module's section |
| GET | `/modules` | List modules and their enabled state |
| GET | `/capabilities` | Server-wide feature flags (`{"llm": bool, "llmVision": bool, "bankImport": bool}`) |
| GET/PUT | `/settings` | Settings incl. enabled modules |
| PUT | `/settings/password` | Change password |
| GET/PUT/DELETE | `/paperless/config` | Paperless-ngx connection (per user; token stored encrypted) |
| POST | `/paperless/config/test` | Test the paperless connection |
| GET | `/paperless/search` | Proxied paperless document search |
| GET | `/paperless/documents/{id}/thumb` | Proxied document thumbnail |
| GET/POST | `/paperless/links/{entityType}/{entityId}` | List / attach paperless document links |
| DELETE | `/paperless/links/{entityType}/{entityId}/{documentId}` | Detach a document link |
| GET/PUT | `/paperless/pipeline` | Draft pipeline settings (on/off, inbox and done tag) |
| POST | `/paperless/pipeline/run` | Run the draft pipeline for the current user now |
| GET | `/drafts` | List drafts (`?status=pending` default, `accepted`, `rejected`, `all`) |
| GET | `/drafts/summary` | Number of pending and of all drafts (`{pending, total}`) |
| GET/PUT | `/drafts/{id}` | Get / edit a pending draft (type, target, input) |
| POST | `/drafts/{id}/accept` | Accept a draft after creating its entity (links the source) |
| POST | `/drafts/{id}/reject` | Reject a draft |
| POST | `/quick-add` | Classify a typed note (JSON) or an uploaded PDF or image (`multipart/form-data`: `file`, optional `text`, `targetType`) into a proposed purchase, contract or vehicle cost (nothing stored) |
| GET | `/contracts/summary` | Contract dashboard stats |

Health (`/healthz`), readiness (`/readyz`), and Prometheus metrics (`/metrics`) are available at the root.

Internal transfers are protected from accidental category assignment. To recategorize a linked transfer as a normal transaction, unlink it first.

Routes of modules a user has disabled return `403` with code `module_disabled`; the data stays in the database and everything returns when the module is re-enabled.

## AI Disclaimer

This project was developed with the assistance of AI tools and continues to use AI in its development.

## License

[MIT](LICENSE)
