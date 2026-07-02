import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { autoVehicleDetailRoute } from "./auto.vehicles.$vehicleId"

const AutoIndexPage = lazy(() => import("./auto.index").then((module) => ({ default: module.AutoIndexPage })))

export const autoIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto",
  beforeLoad: moduleGuard("auto"),
  component: AutoIndexPage,
})

export const autoRoutes: AnyRoute[] = [
  autoIndexRoute,
  autoVehicleDetailRoute,
]
