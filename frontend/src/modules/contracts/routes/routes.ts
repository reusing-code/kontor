import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { contractDetailRoute } from "./contracts.$contractId"
import { contractsCategoryRoute } from "./contracts.categories.$categoryId"
import { contractsUpcomingRenewalsRoute } from "./contracts.upcoming-renewals"

const ContractsDashboardPage = lazy(() => import("./contracts.index").then((module) => ({ default: module.ContractsDashboardPage })))

export const contractsIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts",
  component: ContractsDashboardPage,
})

export const contractsRoutes: AnyRoute[] = [
  contractsIndexRoute,
  contractDetailRoute,
  contractsCategoryRoute,
  contractsUpcomingRenewalsRoute,
]
