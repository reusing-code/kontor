import { describe, expect, it } from "vitest"
import { router } from "@/routes/router"
import { ledgerAllTransactionsRoute, ledgerRoutes, ledgerTransactionRoute } from "@/modules/ledger/routes/routes"

describe("ledger transactions route", () => {
  it("is registered next to the transaction detail route", () => {
    expect(ledgerRoutes).toContain(ledgerAllTransactionsRoute)
    expect(router.matchRoutes("/ledger/transactions").at(-1)?.routeId).toBe(ledgerAllTransactionsRoute.id)
    expect(router.matchRoutes("/ledger/transactions/txn-1").at(-1)?.routeId).toBe(ledgerTransactionRoute.id)
  })

  it("keeps the account selection from the search", () => {
    const validate = ledgerAllTransactionsRoute.options.validateSearch as (search: Record<string, unknown>) => unknown
    expect(validate({ accounts: ["a", "b"] })).toEqual({ accounts: ["a", "b"] })
    expect(validate({ accounts: "junk" })).toEqual({})
    expect(validate({ accounts: ["a"], page: 2 })).toEqual({ accounts: ["a"], page: 2 })
  })
})
