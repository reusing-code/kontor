# Kontor — Frontend

React SPA for the Kontor application — a multi-module personal finance manager covering contracts, purchases, vehicles, and ledger transactions. Each module lives in `src/modules/{id}/` and is described by a module registry that drives routing, navigation, and the homepage; modules can be enabled/disabled per account in settings.

## Tech Stack

- **React 19** + **TypeScript** (Vite)
- **Tailwind CSS v4** + **shadcn/ui** for components
- **TanStack Router** for type-safe routing
- **TanStack Query** for server state / data fetching
- **React Hook Form** + **Zod** for forms and validation
- **Recharts** for charts (vehicle statistics, ledger analysis)
- **react-i18next** for internationalization (en/de)

## Getting Started

```bash
bun install
bun dev
```

Dev server runs at `http://localhost:5173`.

## Scripts

| Command | Description |
|---------|-------------|
| `bun dev` | Start dev server with HMR |
| `bun run build` | Type-check and build for production |
| `bun run lint` | Run Oxlint (type-aware, with the jsx-a11y rules) |
| `bun run test` | Run the tests once (Vitest, jsdom) |
| `bun run test:coverage` | Run the tests with a coverage report (v8; summary in the terminal, HTML in `coverage/`) |
| `bunx vitest` | Run the tests in watch mode |
| `bun run preview` | Preview production build |

## Adding UI Components

This project uses [shadcn/ui](https://ui.shadcn.com). Components are not installed as dependencies — they're copied into `src/components/ui/` and can be freely customized.

```bash
npx shadcn@latest add button
npx shadcn@latest add table
```

Browse available components at [ui.shadcn.com](https://ui.shadcn.com).

## Project Structure

```
src/
  routes/        Route definitions (homepage, contracts/*, purchases/*, auto/*, ledger/*)
  components/    React components (ui/ for shadcn)
  lib/           Utilities, API client, per-module repositories
  hooks/         Custom React hooks (categories, contracts, purchases, vehicles, ledger)
  types/         Shared TypeScript types
  config/        Field configurations for forms/tables
  i18n/          Internationalization (en.json, de.json)
```

The `@/` import alias maps to `src/`.

The settings page has an "Access tokens" card (`src/components/tokens/`): create, list and revoke personal access tokens (shown once on creation), with copy-ready setup snippets for MCP clients.

The draft inbox at `/inbox` lists entries proposed from tagged paperless documents, emails in the folder for the Kontor inbox of a ledger email account (checked in the background or with "Check Kontor inbox folders" on the email accounts page; the accepted entry shows its source email; the account dialog can have decided emails moved to another folder or deleted, as it can for processed order emails), recurring payments detected in the ledger (as contracts, after each import or with "Detect recurring payments" on the ledger dashboard) and drafts created by agents through MCP (purchases, contracts, vehicle costs, vehicles, categories, ledger accounts); accepting opens the matching prefilled create dialog, and accepting a recurring payment links its payments to the new contract. Drafts can also be selected (a type filter and "Select all" help with many of them) and accepted in one step without the dialogs; a draft that cannot be created stays in the inbox with the reason. It appears when the server has an LLM configured and paperless is connected, or as soon as any draft exists.

Quick add (header button, Ctrl/Cmd+K) turns a typed note, an attached PDF or (with a vision model on the server) a photo into a prefilled create dialog; it only appears when the server has an LLM configured and at least one of the purchases, contracts or auto modules is enabled.

With the server-side bank import configured (the `bankImport` capability), Ledger → Bank connections connects banks through Enable Banking, maps their accounts to ledger accounts and syncs them; the consent banner on the ledger dashboard warns before a consent ends.

`/ledger/transactions` lists the transactions of all accounts together, optionally narrowed to some accounts with the account filter. Ledger accounts show their current balance (from CSV exports, the bank sync or set by hand on the account page, with a warning when two balances do not add up), and the account's transaction list (100 per page, with previous/next and page number buttons; the page is in the URL) shows the balance after and before each page. Ledger transactions can be reviewed from `/ledger/review`, linked to other modules via references, and explicitly linked/unlinked as internal transfers on the review and detail screens. The detail screen can also create a purchase (one per item of a linked email order), a vehicle cost, a contract or a vehicle prefilled from the transaction and links it right away.
