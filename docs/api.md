# REST API reference

Paths in the tables are relative to `/api/v1`. The `/auth/*` routes and the bank consent callback are public. For other routes, send `Authorization: Bearer <token>` with a login JWT or a [personal access token](mcp.md#token-scopes). A personal token permits only the reads and writes listed for its scopes; other operations require a login JWT. Using a token outside its scopes returns `403` (`insufficient_scope`).

Routes of modules a user has disabled return `403` with code `module_disabled`; the data stays in the database and everything returns when the module is re-enabled.

Contracts, purchases, vehicles, vehicle costs and their categories are validated the same way on every path (REST, MCP drafts, accepting drafts, `POST /contracts/import`): names are trimmed and required, dates are `YYYY-MM-DD`, amounts, mileages and months must not be negative (months at most 1200), and text is capped at 200 characters (URLs 2048, comments 10000). A violation is a `400` naming the field. Importing an export file does not apply these rules.

The server also exposes `/healthz` (liveness), `/readyz` (readiness) and `/metrics` (Prometheus) at the root. Agent clients use the separate [MCP endpoint](mcp.md).

## Authentication and tokens

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/register` | Register user (403 `registration_closed` while registration is closed, see `REGISTRATION_ENABLED`) |
| POST | `/auth/login` | Login (returns JWT) |
| GET | `/auth/config` | Public: `{registrationOpen}` for the login page |
| GET/POST | `/tokens` | List / create personal access tokens (JWT only; the raw token is returned once) |
| DELETE | `/tokens/{id}` | Revoke a personal access token (JWT only) |

## Settings, modules and data portability

| Method | Path | Description |
|--------|------|-------------|
| GET | `/export` | Download all user data (v2 envelope) |
| GET | `/modules/{module}/export` | Download a single module's data |
| POST | `/import` | Import an export into empty, enabled modules |
| POST | `/modules/{module}/import` | Import a single module's section |
| GET | `/modules` | List modules and their enabled state |
| GET | `/capabilities` | Server-wide feature flags (`{"llm": bool, "llmVision": bool, "bankImport": bool}`) |
| GET/PUT | `/settings` | Settings incl. enabled modules |
| PUT | `/settings/password` | Change password (8 characters to 72 bytes); ends every login session and returns `{token}` for the current one |

## Contracts and purchases

| Method | Path | Description |
|--------|------|-------------|
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
| GET | `/contracts/summary` | Contract dashboard stats |

## Vehicles and costs

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/vehicles` | List / create vehicles |
| GET/PUT/DELETE | `/vehicles/{id}` | Vehicle CRUD |
| GET | `/vehicles/{id}/summary` | Vehicle cost summary + projection |
| GET/POST | `/vehicles/{id}/costs` | List / create cost entries |
| GET/PUT/DELETE | `/costs/{id}` | Cost entry CRUD |

## Ledger accounts and imports

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ledger/accounts` | List tracked ledger accounts with their current balance |
| POST | `/ledger/accounts` | Create a ledger account (`{name, bank, iban?, currency?}`, IBAN checksum validated, duplicate IBAN 409; JWT only) |
| GET | `/ledger/accounts/{id}` | Get ledger account details with the current balance |
| PUT | `/ledger/accounts/{id}` | Rename a ledger account (`{name, bank}`; IBAN and currency are fixed) |
| GET | `/ledger/accounts/{id}/delete-summary` | What deleting the account removes (transactions, imports, links, mapped bank accounts) |
| DELETE | `/ledger/accounts/{id}` | Delete a ledger account with its transactions and imports |
| GET | `/ledger/accounts/{id}/transactions` | List transactions for an account (`limit` + `offset`, with the `total`), with the balance after and before the page |
| POST | `/ledger/accounts/{id}/balance` | Set the account balance by hand (`{amountMinor, asOf?}`; JWT only) |
| DELETE | `/ledger/accounts/{id}/balance/{asOf}` | Delete a balance set by hand (JWT only) |
| GET | `/ledger/imports` | List ledger import batches |
| POST | `/ledger/imports/preview` | Preview a ledger import file |
| POST | `/ledger/imports/{previewId}/commit` | Commit a previewed ledger import |

## Bank connections

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ledger/bank/aspsps` | Banks Enable Banking offers for `?country=DE` (JWT only) |
| GET | `/ledger/bank/connections` | Bank connections with account mapping and sync status (never the session) |
| POST | `/ledger/bank/connections/authorize` | Start a bank consent (`{aspspName, aspspCountry}` → `{url}`) |
| DELETE | `/ledger/bank/connections/{id}` | Disconnect a bank (`{remoteClosed}`) |
| PUT | `/ledger/bank/connections/{id}/accounts/{uid}` | Map a bank account to a ledger account (`{ledgerAccountId}`), also to change an existing mapping |
| DELETE | `/ledger/bank/connections/{id}/accounts/{uid}` | Unmap a bank account (not synced and not mapped by IBAN until it is mapped again) |
| POST | `/ledger/bank/connections/{id}/sync` | Sync a connection now (409 `sync_running` / `consent_expired`) |
| GET | `/ledger/bank/callback` | Public redirect target of the consent (state is the credential; answers `303` to the result page) |

## Transactions, categories and review

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ledger/transactions` | List the transactions of all accounts, newest first (`limit` + `offset`, with the `total`; repeatable `accountId` to narrow to some accounts) |
| GET | `/ledger/review-queue` | Review queue for ledger transactions (`limit` + `cursor`, or `limit=all` for the whole queue) |
| GET | `/ledger/transactions/{id}` | Get ledger transaction details |
| PUT | `/ledger/transactions/{id}` | Update note, links, and cross references |
| POST | `/ledger/transactions/{id}/references` | Add one cross reference (`{type, targetId}`, type `purchase`, `contract`, `vehicle` or `cost`; idempotent, never removes anything; `links` scope) |
| GET | `/ledger/transactions/{id}/transfer-candidates` | List matching internal transfer candidates |
| POST | `/ledger/transactions/{id}/transfer-link` | Link two transactions as an internal transfer |
| DELETE | `/ledger/transactions/{id}/transfer-link` | Explicitly unlink an internal transfer |
| POST | `/ledger/transactions/{id}/review` | Confirm or categorize a ledger transaction |
| PUT | `/ledger/transactions/{id}/splits` | Split a transaction into parts with their own category (empty list removes the split) |
| POST | `/ledger/transactions/review-batch` | Confirm up to 100 transactions with a category each (accept suggestions); per-item failures |
| POST | `/ledger/category-suggestions/run` | Ask the LLM for category suggestions now (409 without LLM or while a run is going) |
| GET | `/ledger/category-suggestions/status` | `{running}` for the current user |
| POST | `/ledger/recurring/run` | Detect recurring payments now and propose new groups as contract drafts (`{groups, created, skipped}`; 409 while a run is going, 403 `module_disabled` when contracts are disabled) |
| GET/POST | `/ledger/categories` | List / create ledger categories |
| GET/PUT/DELETE | `/ledger/categories/{id}` | Ledger category CRUD |

Internal transfers are protected from accidental category assignment. To recategorize a linked transfer as a normal transaction, unlink it first. After every import Kontor links clear-cut transfers between your accounts by itself: two unreviewed transactions with opposite amounts on two of your accounts within three days that have no other possible match, and where one names the other account's IBAN or both show the same counterparty name. Reviewed transactions are left alone.

## Ledger analysis

| Method | Path | Description |
|--------|------|-------------|
| GET | `/ledger/analysis/spending` | Monthly spending by category (`from`, `to`, optional `accountId`; `parentCategoryId` for the subcategories of one top-level category) |
| GET | `/ledger/analysis/cash-flow` | Monthly income, expenses and net result (`from`, `to`) |
| GET | `/ledger/analysis/ranking` | Top categories and counterparties by spending in the range (`from`, `to`) |
| GET | `/ledger/analysis/category-breakdown` | Spending of the range per top-level category and its direct subcategories, netted over the range (`from`, `to`) |
| GET | `/ledger/analysis/net-worth` | Net worth over time: month-end balances per account and in total, with the accounts left out (`from`, `to`) |

## Email accounts and orders

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/ledger/email-accounts` | List / create ledger email accounts (`folder`, optional `draftsFolder`, and per folder the action for processed emails: `processedAction` / `processedFolder`, `draftsProcessedAction` / `draftsProcessedFolder` with `keep`, `move`, `delete`) |
| GET/PUT/DELETE | `/ledger/email-accounts/{emailAccountId}` | Ledger email account CRUD |
| POST | `/ledger/email-accounts/{emailAccountId}/test` | Test the configured IMAP connection; 502 `imap_login_failed` / `imap_folder_failed` / `imap_unavailable` with fixed texts (the mail server's answer is never returned) |
| POST | `/ledger/email-accounts/folders` | List the IMAP folders of a connection as entered in the account dialog (`{imapHost, imapPort, username, useTls, password?, accountId?}`; without a password the stored one of `accountId` is used) → `{folders}`. 409 `password_missing`, 502 `imap_login_failed` / `imap_unavailable` |
| POST | `/ledger/email-accounts/{emailAccountId}/scan` | Start a background scan of the account's folder via IMAP with the LLM (202 with the scan status), matching resulting orders to ledger transactions; a multipart upload of `.eml` files is processed within the request instead (200 with the result). 409 when the LLM is off, no folder or password is set or a scan is already running |
| GET | `/ledger/email-accounts/{emailAccountId}/scan` | Status of the running or last scan of the account: `state` (`idle`, `running`, `done`, `partial`, `failed`), counts, warnings |
| POST | `/ledger/email-drafts/run` | Check the Kontor inbox folders of the email accounts now and propose new messages as drafts (`{found, created, skipped, failed, warnings}`; 409 without the LLM, without an enabled purchases/contracts/auto module or while a check is running) |
| GET | `/ledger/email-links/{entityType}/{entityId}` | Emails a contract, purchase or vehicle cost was created from (entityType `contract`, `purchase`, `cost`) |
| GET | `/ledger/email-orders` | List extracted email orders |
| GET | `/ledger/email-orders/{emailOrderId}` | Get extracted email order details |
| GET | `/ledger/email-orders/{emailOrderId}/candidates` | List unlinked outgoing transactions within the charge window derived from the order's merchant and payment method (`?afterDays=` widens it); for orders in another currency with `conversions` (the total per account currency at the ECB rate) or `unconvertedCurrencies`, and `amountComparable` per item |
| POST | `/ledger/email-orders/{emailOrderId}/link` | Manually link an email order to one or more ledger transactions |
| POST | `/ledger/email-orders/{emailOrderId}/reject` | Reject an extracted email order |

## Paperless documents

| Method | Path | Description |
|--------|------|-------------|
| GET/PUT/DELETE | `/paperless/config` | Paperless-ngx connection (per user; token stored encrypted) |
| POST | `/paperless/config/test` | Test the paperless connection |
| GET | `/paperless/search` | Proxied paperless document search |
| GET | `/paperless/documents/{id}/thumb` | Proxied document thumbnail |
| GET | `/paperless/links/{entityType}` | All paperless document links of one entity type (for the list indicators) |
| GET/POST | `/paperless/links/{entityType}/{entityId}` | List / attach paperless document links |
| DELETE | `/paperless/links/{entityType}/{entityId}/{documentId}` | Detach a document link |
| GET/PUT | `/paperless/pipeline` | Draft pipeline settings (on/off, inbox and done tag) |
| POST | `/paperless/pipeline/run` | Run the draft pipeline for the current user now |

## Drafts and quick add

| Method | Path | Description |
|--------|------|-------------|
| GET | `/drafts` | List drafts (`?status=pending` default, `accepted`, `rejected`, `all`) |
| GET | `/drafts/summary` | Number of pending and of all drafts (`{pending, total}`) |
| GET/PUT | `/drafts/{id}` | Get / edit a pending draft (type, target, input) |
| POST | `/drafts/{id}/accept` | Accept a draft after creating its entity (links the source) |
| POST | `/drafts/accept-batch` | Create the entries of up to 100 drafts and accept them (`{ids, baseUrl}` → one result per draft: `accepted`, `already_accepted` or `failed` with `code` and `error`; failed drafts stay pending; login only, no access token) |
| POST | `/drafts/{id}/reject` | Reject a draft |
| POST | `/quick-add` | Classify a typed note (JSON) or an uploaded PDF or image (`multipart/form-data`: `file`, optional `text`, `targetType`) into a proposed purchase, contract or vehicle cost (nothing stored) |
