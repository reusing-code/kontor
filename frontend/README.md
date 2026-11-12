# Kontor — Frontend

React SPA for the Kontor application — a multi-module personal finance manager covering contracts, purchases, vehicles, and ledger transactions. Each module lives in `src/modules/{id}/` and is described by a module registry that drives routing, navigation, and the homepage; modules can be enabled/disabled per account in settings.

## Tech Stack

- **React 19** + **TypeScript** (Vite)
- **Tailwind CSS v4** + **shadcn/ui** for components
- **TanStack Router** for type-safe routing
- **TanStack Query** for server state / data fetching
- **React Hook Form** + **Zod** for forms and validation
- **Recharts** for charts (vehicle statistics)
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
| `bun run lint` | Run Oxlint (type-aware) |
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

The draft inbox at `/inbox` lists entries proposed from tagged paperless documents and drafts created by agents through MCP (purchases, contracts, vehicle costs, vehicles, categories, ledger accounts); accepting opens the matching prefilled create dialog. It appears when the server has an LLM configured and paperless is connected, or as soon as any draft exists.

Quick add (header button, Ctrl/Cmd+K) turns a typed note into a prefilled create dialog; it only appears when the server has an LLM configured and at least one of the purchases, contracts or auto modules is enabled.

Ledger transactions can be reviewed from `/ledger/review`, linked to other modules via references, and explicitly linked/unlinked as internal transfers on the review and detail screens.
