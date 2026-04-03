# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Commands

- `bun dev` — Start dev server (Vite, port 5173)
- `bun run build` — Type-check with `tsc -b` then Vite production build
- `bun run lint` — ESLint
- `bun run preview` — Preview production build locally
- `npx shadcn@latest add <component>` — Add a shadcn/ui component (e.g. button, table, dialog)

## Architecture

React 19 + TypeScript SPA built with Vite. No SSR — this is a CRUD/business app behind auth.

**Routing:** TanStack Router with type-safe route definitions in `src/routes/`. The router is created in `router.ts`, root layout in `__root.tsx`. Register the router type via the `Register` interface in `router.ts`. Routes use file-based conventions with dots for nesting (e.g. `contracts.index.tsx`, `contracts.categories.$categoryId.tsx`). All routes use `rootRoute` as parent with full paths (flat structure, no nested layout routes).

**Data fetching:** TanStack Query. `QueryClientProvider` wraps the app in `App.tsx`. Hooks in `src/hooks/` wrap query/mutation logic per module (contracts, purchases, categories, vehicles).

**Forms:** React Hook Form + Zod for validation via `@hookform/resolvers`. Field configs in `src/config/` drive both form and table rendering via `FormFieldRenderer`.

**Styling:** Tailwind CSS v4 with the Vite plugin. shadcn/ui for pre-built components (config in `components.json`, components go in `src/components/ui/`). Use the `cn()` helper from `@/lib/utils` for conditional classNames.

**Path alias:** `@/` maps to `src/` (configured in both tsconfig and vite.config).

**i18n:** `react-i18next` with locale files in `src/i18n/locales/` (en.json, de.json).

## Key directories

- `src/routes/` — Route definitions (pages): homepage, contracts/*, purchases/*, auto/*
- `src/components/` — React components; `ui/` subdirectory for shadcn/ui
- `src/lib/` — Utilities, API client, per-module repositories (category, contract, purchase, vehicle)
- `src/hooks/` — Custom React hooks (use-categories, use-contracts, use-purchases, use-vehicles)
- `src/types/` — Shared TypeScript types (contract, purchase, category, vehicle, summary)
- `src/config/` — Field configuration for forms/tables (contract-fields, purchase-fields, vehicle-fields)
- `src/i18n/` — Internationalization setup and locale files
