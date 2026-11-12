# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Documentation maintenance

After every change, check whether `AGENTS.md` (root, `backend/`, `frontend/`) and `README.md` (root, `frontend/`) need updating. Keep these files in sync with the actual code — new modules, routes, DB keys, API endpoints, and dependency versions must be reflected here.

## Commands

- `bun dev` — Start dev server (Vite, port 5173)
- `bun run build` — Type-check with `tsc -b` then Vite production build
- `bun run lint` — Oxlint with type-aware rules (`oxlint --type-aware`, config in `.oxlintrc.json`); TypeScript 7 (native `tsc`) means typescript-eslint is not used
- `bun run preview` — Preview production build locally
- `npx shadcn@latest add <component>` — Add a shadcn/ui component (e.g. button, table, dialog)

## Architecture

React 19 + TypeScript 7 SPA built with Vite 8. No SSR — this is a CRUD/business app behind auth.

**Modules:** Each feature module lives in `src/modules/{id}/` (routes, components, hooks, lib/repository, config, types.ts) and exports a `ModuleDefinition` (`src/types/modules.ts`): id, basePath, i18n label key, icon, route objects, a `SidebarSection`, and an optional `HomeCard`. `src/modules/registry.ts` lists all modules in order; the router, sidebar, and homepage are driven by it. Users enable/disable modules in settings — `useModules()` (`src/hooks/use-modules.ts`) derives enablement from the shared `["settings"]` query, the sidebar and homepage only mount enabled modules, and every module route has a `beforeLoad` guard (`src/modules/guard.ts`) that redirects to `/` when the module is disabled.

**Routing:** TanStack Router with type-safe route definitions. The router is created in `src/routes/router.ts` from core routes (homepage, login, settings) plus each module's routes; root layout in `src/routes/__root.tsx`. Route objects are defined next to their page components in `src/modules/{id}/routes/` and collected in that module's `routes/routes.ts`. All routes use `rootRoute` as parent with full paths (flat structure, no nested layout routes).

**Data fetching:** TanStack Query. The shared `QueryClient` lives in `src/lib/query-client.ts` (used by both `App.tsx` and route guards). Hooks live with their module (`src/modules/{id}/hooks/`); shared hooks (auth, settings, categories, modules) in `src/hooks/`.

**Capabilities:** `useCapabilities()` (`src/hooks/use-capabilities.ts`) reads server-wide feature flags from `GET /api/v1/capabilities` (`staleTime: Infinity`, they only change with a server restart). LLM-backed UI must hide its actions while `llm` is false (the default until the query resolves). The ledger email screens are the exception that shows a notice instead: `LedgerEmailLlmNotice` explains that scanning needs the LLM, the scan and upload buttons are disabled and the account dialog hides the pre-classification switch.

**Draft inbox:** `/inbox` (`src/routes/inbox.tsx`, components in `src/components/drafts/`, `useInboxAvailable`/`usePendingDrafts` in `src/hooks/use-drafts.ts`) lists pending drafts. It and its sidebar entry (with pending-count badge) only exist when `llm` is true and paperless is configured; the route's `beforeLoad` redirects to `/` otherwise. Type and target pickers only offer enabled modules. Accept opens the module's create dialog with `initialValues` (`PurchaseDialog`, `ContractDialog`, `CostEntryDialog`; pass a memoized object), creates the entity through the module hook, then calls the accept endpoint and shows returned warnings as toasts. Draft `note` codes are translated via `drafts.notes.*`. The paperless settings card shows the pipeline section (switch, tag names, Run now) only when `llm` is true.

**Quick add:** `QuickAdd` (`src/components/quick-add/quick-add.tsx`) sits in the root layout header: button plus Ctrl/Cmd+K, rendered only when `useQuickAddTypes()` (`src/hooks/use-quick-add.ts`; `llm` capability and enabled purchases/contracts/auto) is non-empty. `QuickAddDialog` posts the note to `/api/v1/quick-add`; a `confident` proposal whose type and category/vehicle pass `targetProblem` goes straight to `PrefilledCreateDialogs`, otherwise it shows `TargetPicker` with the translated note (`quickAdd.notes.*`) or currency hint; picking another type re-runs the request with `targetType`. After creating, a toast offers "Open" (`entityPath`). The typed text is kept until an entry is created. Shared with the inbox (`src/components/drafts/`): `TargetPicker`, `useParentOptions`, `PrefilledCreateDialogs`, and `draft-input.ts` (`initialValues`, `targetProblem`, `entityPath`/`entityUrl`, `PrefillEntry`).

**Forms:** React Hook Form + Zod for validation via `@hookform/resolvers`. Field configs in `src/modules/{id}/config/` drive both form and table rendering via `FormFieldRenderer`.

**Styling:** Tailwind CSS v4 with the Vite plugin. shadcn/ui for pre-built components (config in `components.json`, components go in `src/components/ui/`). Use the `cn()` helper from `@/lib/utils` for conditional classNames.

**Charts:** Recharts, currently used by the auto module's vehicle statistics page (`/auto/vehicles/$vehicleId/statistics`, lazy-loaded). Cost-type series colors are CSS custom properties (`--viz-*`) defined for light and dark mode in `src/index.css` and mapped in `src/modules/auto/config/chart.ts`.

**Path alias:** `@/` maps to `src/` (configured in both tsconfig and vite.config).

**i18n:** `react-i18next` with locale files in `src/i18n/locales/` (en.json, de.json).

**Import/export:** The settings page offers full export/import plus per-module export/import against `/api/v1/export`, `/api/v1/import`, and `/api/v1/modules/{id}/export|import` (v2 envelope format).

## Key directories

- `src/modules/{contracts,purchases,auto,ledger}/` — Per-module routes, components, hooks, repository, field configs, and types; `index.tsx` exports the ModuleDefinition
- `src/modules/registry.ts` — Ordered module registry; `src/modules/guard.ts` — route enablement guard
- `src/routes/` — Core pages: homepage, login, settings, inbox, root layout, router
- `src/components/` — Shared components (sidebar, category dialogs, linked-transactions list); `ui/` for shadcn/ui; `paperless/` for the paperless-ngx integration (settings card with pipeline section, search dialog, documents section, thumbnail) — all paperless surfaces render nothing until the integration is configured; `drafts/` for the inbox and shared create flow (draft card, target picker, parent options, prefilled create dialogs, input mapping); `quick-add/` for the header quick add
- `src/hooks/` — Shared hooks (use-auth, use-settings, use-modules, use-categories, use-page-title, use-paperless, use-capabilities, use-drafts, use-quick-add)
- `src/lib/` — API client, query client, shared repositories (auth, settings, categories, paperless, capabilities, drafts, quick add), utils
- `src/types/` — Shared types (auth, category, settings, modules, paperless, capabilities, drafts, quick-add)
- `src/i18n/` — Internationalization setup and locale files

Ledger transaction detail and review flows include explicit internal transfer linking/unlinking. Keep that behavior visible in the UI and avoid implicit unlinking when editing unrelated fields. Cross-module reference UI (linking transactions to contracts/purchases/vehicles) must only offer targets from enabled modules and render references to disabled modules inert.
