import { createRouter } from "@tanstack/react-router"
import { rootRoute } from "./__root"
import { indexRoute } from "."
import { contractsIndexRoute } from "./contracts.index"
import { contractsCategoryRoute } from "./contracts.categories.$categoryId"
import { contractsUpcomingRenewalsRoute } from "./contracts.upcoming-renewals"
import { purchasesIndexRoute } from "./purchases.index"
import { purchasesCategoryRoute } from "./purchases.categories.$categoryId"
import { autoIndexRoute } from "./auto.index"
import { autoVehicleDetailRoute } from "./auto.vehicles.$vehicleId"
import { ledgerIndexRoute } from "./ledger.index"
import { ledgerAccountRoute } from "./ledger.accounts.$accountId"
import { loginRoute } from "./login"
import { settingsRoute } from "./settings"

const routeTree = rootRoute.addChildren([
  indexRoute,
  contractsIndexRoute,
  contractsCategoryRoute,
  contractsUpcomingRenewalsRoute,
  purchasesIndexRoute,
  purchasesCategoryRoute,
  autoIndexRoute,
  autoVehicleDetailRoute,
  ledgerIndexRoute,
  ledgerAccountRoute,
  loginRoute,
  settingsRoute,
])

export const router = createRouter({ routeTree })

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}
