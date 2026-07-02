import { lazy } from "react"
import { createRoute, createRouter } from "@tanstack/react-router"
import { rootRoute } from "./__root"
import { indexRoute } from "./index"
import { loginRoute } from "./login"
import { modules } from "@/modules/registry"

const SettingsPage = lazy(() => import("./settings").then((module) => ({ default: module.SettingsPage })))

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  ...modules.flatMap((m) => m.routes),
  loginRoute,
  settingsRoute,
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}
