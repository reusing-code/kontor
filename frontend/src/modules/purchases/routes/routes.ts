import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { purchaseDetailRoute } from "./purchases.$purchaseId"
import { purchasesCategoryRoute } from "./purchases.categories.$categoryId"

const PurchasesDashboardPage = lazy(() => import("./purchases.index").then((module) => ({ default: module.PurchasesDashboardPage })))

export const purchasesIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases",
  beforeLoad: moduleGuard("purchases"),
  component: PurchasesDashboardPage,
})

export const purchasesRoutes: AnyRoute[] = [
  purchasesIndexRoute,
  purchaseDetailRoute,
  purchasesCategoryRoute,
]
