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

**Modules:** Each feature module lives in `src/modules/{id}/` (routes, components, hooks, lib/repository, config, types.ts) and exports a `ModuleDefinition` (`src/types/modules.ts`): id, basePath, i18n label key, icon, route objects, a `SidebarSection`, and an optional `HomeCard`. `src/modules/registry.ts` lists all modules in order; the router, sidebar, and homepage are driven by it. Users enable/disable modules in settings — `useModules()` (`src/hooks/use-modules.ts`) derives enablement from the shared `["settings"]` query, the sidebar and homepage only mount enabled modules, and every module route has a `beforeLoad` guard (`src/modules/guard.ts`) that redirects to `/` when the module is disabled.

**Routing:** TanStack Router with type-safe route definitions. The router is created in `src/routes/router.ts` from core routes (homepage, login, settings) plus each module's routes; root layout in `src/routes/__root.tsx`. Route objects are defined next to their page components in `src/modules/{id}/routes/` and collected in that module's `routes/routes.ts`. All routes use `rootRoute` as parent with full paths (flat structure, no nested layout routes).

**Data fetching:** TanStack Query. The shared `QueryClient` lives in `src/lib/query-client.ts` (used by both `App.tsx` and route guards). Hooks live with their module (`src/modules/{id}/hooks/`); shared hooks (auth, settings, categories, modules) in `src/hooks/`.

**Forms:** React Hook Form + Zod for validation via `@hookform/resolvers`. Field configs in `src/modules/{id}/config/` drive both form and table rendering via `FormFieldRenderer`.

**Styling:** Tailwind CSS v4 with the Vite plugin. shadcn/ui for pre-built components (config in `components.json`, components go in `src/components/ui/`). Use the `cn()` helper from `@/lib/utils` for conditional classNames.

**Charts:** Recharts, currently used by the auto module's vehicle statistics page (`/auto/vehicles/$vehicleId/statistics`, lazy-loaded). Cost-type series colors are CSS custom properties (`--viz-*`) defined for light and dark mode in `src/index.css` and mapped in `src/modules/auto/config/chart.ts`.

**Path alias:** `@/` maps to `src/` (configured in both tsconfig and vite.config).

**i18n:** `react-i18next` with locale files in `src/i18n/locales/` (en.json, de.json).

**Import/export:** The settings page offers full export/import plus per-module export/import against `/api/v1/export`, `/api/v1/import`, and `/api/v1/modules/{id}/export|import` (v2 envelope format).

## Key directories

- `src/modules/{contracts,purchases,auto,ledger}/` — Per-module routes, components, hooks, repository, field configs, and types; `index.tsx` exports the ModuleDefinition
- `src/modules/registry.ts` — Ordered module registry; `src/modules/guard.ts` — route enablement guard
- `src/routes/` — Core pages: homepage, login, settings, root layout, router
- `src/components/` — Shared components (sidebar, category dialogs, linked-transactions list); `ui/` for shadcn/ui
- `src/hooks/` — Shared hooks (use-auth, use-settings, use-modules, use-categories, use-page-title)
- `src/lib/` — API client, query client, shared repositories (auth, settings, categories), utils
- `src/types/` — Shared types (auth, category, settings, modules)
- `src/i18n/` — Internationalization setup and locale files

Ledger transaction detail and review flows include explicit internal transfer linking/unlinking. Keep that behavior visible in the UI and avoid implicit unlinking when editing unrelated fields. Cross-module reference UI (linking transactions to contracts/purchases/vehicles) must only offer targets from enabled modules and render references to disabled modules inert.
