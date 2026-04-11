# Contracts

A self-hosted personal finance manager for tracking contracts, subscriptions, purchases, vehicle costs, and ledger transactions. Built with a Go backend and React frontend.

## Features

- **Modular design** — Separate modules for contracts, purchases, and auto/vehicles, each with independent dashboards
- **Contract tracking** — Store contract details including dates, pricing, notice periods, and renewal terms
- **Purchase tracking** — Track one-time purchases with item details, pricing, dealer info, and document links
- **Vehicle cost tracking** — Manage vehicles with cost entries (service, fuel, insurance, tax, inspection, tires, mileage, misc) and total cost of ownership projections
- **Ledger module** — Import bank CSVs into tracked accounts, review transactions, manage categories, add notes/links/references, mark internal transfers, and enrich transactions with parsed email order data
- **Email order enrichment** — Configure IMAP email accounts for supported importers like Amazon.de and PayPal.de to scan inbox messages and auto-link parsed orders to ledger transactions; `.eml` upload remains available as a fallback
- **Category organization** — Per-module categories (e.g. insurance/telecom for contracts, PC hardware/tools for purchases)
- **Homepage overview** — Dashboard at `/` with summary cards and stats across all modules
- **Renewal monitoring** — Upcoming renewals with color-coded urgency indicators
- **Email reminders** — Configurable SMTP-based reminder emails for approaching renewals
- **Batch import** — Import contracts from JSON via file upload or paste
- **Multi-user** — JWT authentication with per-user data isolation
- **Observability** — Prometheus metrics, structured logging, health/readiness probes

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Frontend | React 19, TypeScript, Vite, TanStack Router/Query, shadcn/ui, Tailwind CSS |
| Backend | Go (stdlib `net/http`), BadgerDB, JWT, slog |
| Runtime | Docker, Alpine Linux |

## Quick Start

### Prerequisites

- [Go 1.26+](https://go.dev/dl/)
- [Bun](https://bun.sh)
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

## Project Structure

```
frontend/          React SPA (Vite, TanStack, shadcn/ui)
backend/           Go API server (net/http, BadgerDB)
Dockerfile         Multi-stage build (Bun → Go → Alpine)
compose.yml        Single-service deployment with named volume
```

See `frontend/AGENTS.md` and `backend/AGENTS.md` for per-project details.

## API

All endpoints under `/api/v1/`. Auth endpoints are public; everything else requires a JWT bearer token.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/register` | Register user |
| POST | `/auth/login` | Login (returns JWT) |
| GET | `/ledger/accounts` | List tracked ledger accounts |
| GET | `/ledger/accounts/{id}` | Get ledger account details |
| GET | `/ledger/accounts/{id}/transactions` | List transactions for an account |
| GET | `/ledger/transactions` | Review queue for ledger transactions |
| GET | `/ledger/transactions/{id}` | Get ledger transaction details |
| PUT | `/ledger/transactions/{id}` | Update note, links, and cross references |
| GET | `/ledger/transactions/{id}/transfer-candidates` | List matching internal transfer candidates |
| POST | `/ledger/transactions/{id}/transfer-link` | Link two transactions as an internal transfer |
| DELETE | `/ledger/transactions/{id}/transfer-link` | Explicitly unlink an internal transfer |
| POST | `/ledger/transactions/{id}/review` | Confirm or categorize a ledger transaction |
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
| POST | `/ledger/email-accounts/{emailAccountId}/scan` | Parse uploaded `.eml` files and match resulting orders to ledger transactions |
| GET | `/ledger/email-orders` | List parsed email orders |
| GET | `/ledger/email-orders/{emailOrderId}` | Get parsed email order details |
| POST | `/ledger/email-orders/{emailOrderId}/link` | Manually link an email order to one or more ledger transactions |
| POST | `/ledger/email-orders/{emailOrderId}/reject` | Reject a parsed email order |
| GET | `/ledger/email-importers` | List supported email importers |
| GET/PUT | `/settings` | Renewal preferences |
| PUT | `/settings/password` | Change password |
| GET | `/summary` | Contract dashboard stats |

Health (`/healthz`), readiness (`/readyz`), and Prometheus metrics (`/metrics`) are available at the root.

Internal transfers are protected from accidental category assignment. To recategorize a linked transfer as a normal transaction, unlink it first.

## AI Disclaimer

This project was developed with the assistance of AI tools and continues to use AI in its development.

## License

[MIT](LICENSE)
