import { lazy } from "react"
import { createRoute, createRouter, isRedirect, redirect } from "@tanstack/react-router"
import { rootRoute } from "./__root"
import { indexRoute } from "./index"
import { loginRoute } from "./login"
import { modules } from "@/modules/registry"
import { queryClient } from "@/lib/query-client"
import { capabilitiesQueryOptions } from "@/hooks/use-capabilities"
import { paperlessConfigQueryOptions } from "@/hooks/use-paperless"
import { draftSummaryQueryOptions } from "@/hooks/use-drafts"

const SettingsPage = lazy(() => import("./settings").then((module) => ({ default: module.SettingsPage })))

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

const InboxPage = lazy(() => import("./inbox").then((module) => ({ default: module.InboxPage })))

export const inboxRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/inbox",
  beforeLoad: async () => {
    try {
      const [capabilities, paperless] = await Promise.all([
        queryClient.ensureQueryData(capabilitiesQueryOptions),
        queryClient.ensureQueryData(paperlessConfigQueryOptions),
      ])
      if (!capabilities.llm || !paperless.configured) {
        // Drafts created by agents keep the inbox reachable; fetch fresh so a
        // draft that just arrived opens the page.
        const summary = await queryClient.fetchQuery({ ...draftSummaryQueryOptions, staleTime: 0 })
        if (summary.total === 0) {
          throw redirect({ to: "/" })
        }
      }
    } catch (error) {
      if (isRedirect(error)) {
        throw error
      }
    }
  },
  component: InboxPage,
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  ...modules.flatMap((m) => m.routes),
  loginRoute,
  settingsRoute,
  inboxRoute,
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}
