import { lazy } from "react"
import { createRoute, type AnyRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"

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

export const ledgerRoutes: AnyRoute[] = [
  ledgerIndexRoute,
  ledgerAccountRoute,
  ledgerCategoriesRoute,
  ledgerEmailAccountsRoute,
  ledgerEmailOrdersRoute,
  ledgerReviewRoute,
  ledgerTransactionRoute,
]
