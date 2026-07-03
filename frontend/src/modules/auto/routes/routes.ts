import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { autoVehicleDetailRoute } from "./auto.vehicles.$vehicleId"

const AutoIndexPage = lazy(() => import("./auto.index").then((module) => ({ default: module.AutoIndexPage })))
const AutoVehicleStatisticsPage = lazy(() =>
  import("./auto.vehicles.$vehicleId.statistics").then((module) => ({ default: module.AutoVehicleStatisticsPage })),
)

export const autoIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto",
  beforeLoad: moduleGuard("auto"),
  component: AutoIndexPage,
})

export const autoVehicleStatisticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto/vehicles/$vehicleId/statistics",
  beforeLoad: moduleGuard("auto"),
  component: AutoVehicleStatisticsPage,
})

export const autoRoutes: AnyRoute[] = [
  autoIndexRoute,
  autoVehicleDetailRoute,
  autoVehicleStatisticsRoute,
]
