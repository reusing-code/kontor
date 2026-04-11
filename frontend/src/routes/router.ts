import { lazy } from "react"
import { createRoute, createRouter } from "@tanstack/react-router"
import { rootRoute } from "./__root"
import { indexRoute } from "."
import { contractsCategoryRoute } from "./contracts.categories.$categoryId"
import { contractDetailRoute } from "./contracts.$contractId"
import { contractsUpcomingRenewalsRoute } from "./contracts.upcoming-renewals"
import { purchaseDetailRoute } from "./purchases.$purchaseId"
import { purchasesCategoryRoute } from "./purchases.categories.$categoryId"
import { autoVehicleDetailRoute } from "./auto.vehicles.$vehicleId"
import { loginRoute } from "./login"

const ContractsDashboardPage = lazy(() => import("./contracts.index").then((module) => ({ default: module.ContractsDashboardPage })))
const PurchasesDashboardPage = lazy(() => import("./purchases.index").then((module) => ({ default: module.PurchasesDashboardPage })))
const AutoIndexPage = lazy(() => import("./auto.index").then((module) => ({ default: module.AutoIndexPage })))
const LedgerIndexPage = lazy(() => import("./ledger.index").then((module) => ({ default: module.LedgerIndexPage })))
const LedgerAccountPage = lazy(() => import("./ledger.accounts.$accountId").then((module) => ({ default: module.LedgerAccountPage })))
const LedgerCategoriesPage = lazy(() => import("./ledger.categories").then((module) => ({ default: module.LedgerCategoriesPage })))
const LedgerEmailAccountsPage = lazy(() => import("./ledger.email-accounts").then((module) => ({ default: module.LedgerEmailAccountsPage })))
const LedgerEmailOrdersPage = lazy(() => import("./ledger.email-orders").then((module) => ({ default: module.LedgerEmailOrdersPage })))
const LedgerReviewPage = lazy(() => import("./ledger.review").then((module) => ({ default: module.LedgerReviewPage })))
const LedgerTransactionPage = lazy(() => import("./ledger.transactions.$transactionId").then((module) => ({ default: module.LedgerTransactionPage })))
const SettingsPage = lazy(() => import("./settings").then((module) => ({ default: module.SettingsPage })))

export const contractsIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts",
  component: ContractsDashboardPage,
})

export const purchasesIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases",
  component: PurchasesDashboardPage,
})

export const autoIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto",
  component: AutoIndexPage,
})

export const ledgerIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger",
  component: LedgerIndexPage,
})

export const ledgerAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/accounts/$accountId",
  component: LedgerAccountPage,
})

export const ledgerCategoriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/categories",
  component: LedgerCategoriesPage,
})

export const ledgerEmailAccountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/email-accounts",
  component: LedgerEmailAccountsPage,
})

export const ledgerEmailOrdersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/email-orders",
  component: LedgerEmailOrdersPage,
})

export const ledgerReviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/review",
  component: LedgerReviewPage,
})

export const ledgerTransactionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/transactions/$transactionId",
  component: LedgerTransactionPage,
})

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

const routeTree = rootRoute.addChildren([
  indexRoute,
  contractsIndexRoute,
  contractDetailRoute,
  contractsCategoryRoute,
  contractsUpcomingRenewalsRoute,
  purchasesIndexRoute,
  purchaseDetailRoute,
  purchasesCategoryRoute,
  autoIndexRoute,
  autoVehicleDetailRoute,
  ledgerIndexRoute,
  ledgerAccountRoute,
  ledgerCategoriesRoute,
  ledgerEmailAccountsRoute,
  ledgerEmailOrdersRoute,
  ledgerReviewRoute,
  ledgerTransactionRoute,
  loginRoute,
  settingsRoute,
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}
