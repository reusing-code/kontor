import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"

const LedgerIndexPage = lazy(() => import("./ledger.index").then((module) => ({ default: module.LedgerIndexPage })))
const LedgerAccountPage = lazy(() => import("./ledger.accounts.$accountId").then((module) => ({ default: module.LedgerAccountPage })))
const LedgerCategoriesPage = lazy(() => import("./ledger.categories").then((module) => ({ default: module.LedgerCategoriesPage })))
const LedgerEmailAccountsPage = lazy(() => import("./ledger.email-accounts").then((module) => ({ default: module.LedgerEmailAccountsPage })))
const LedgerEmailOrdersPage = lazy(() => import("./ledger.email-orders").then((module) => ({ default: module.LedgerEmailOrdersPage })))
const LedgerReviewPage = lazy(() => import("./ledger.review").then((module) => ({ default: module.LedgerReviewPage })))
const LedgerTransactionPage = lazy(() => import("./ledger.transactions.$transactionId").then((module) => ({ default: module.LedgerTransactionPage })))

export const ledgerIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerIndexPage,
})

export const ledgerAccountRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/accounts/$accountId",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerAccountPage,
})

export const ledgerCategoriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/categories",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerCategoriesPage,
})

export const ledgerEmailAccountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/email-accounts",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerEmailAccountsPage,
})

export const ledgerEmailOrdersRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/email-orders",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerEmailOrdersPage,
})

export const ledgerReviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/review",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerReviewPage,
})

export const ledgerTransactionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/transactions/$transactionId",
  beforeLoad: moduleGuard("ledger"),
  component: LedgerTransactionPage,
})

export const ledgerRoutes: AnyRoute[] = [
  ledgerIndexRoute,
  ledgerAccountRoute,
  ledgerCategoriesRoute,
  ledgerEmailAccountsRoute,
  ledgerEmailOrdersRoute,
  ledgerReviewRoute,
  ledgerTransactionRoute,
]
