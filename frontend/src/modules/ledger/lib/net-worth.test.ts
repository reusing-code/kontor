import { describe, expect, it } from "vitest"
import type { LedgerAccount } from "@/modules/ledger/types"
import { netWorthFromAccounts } from "@/modules/ledger/lib/net-worth"

const at = "2026-11-01T10:00:00Z"

function account(name: string, currency: string, currentMinor?: number): LedgerAccount {
  return {
    id: `acc-${name}`,
    name,
    bank: "DKB",
    currency,
    createdAt: at,
    updatedAt: at,
    balance: currentMinor === undefined ? undefined : { currentMinor, snapshotMinor: currentMinor, asOf: "2026-11-01", source: "dkb.csv" },
  }
}

describe("netWorthFromAccounts", () => {
  it("sums the EUR accounts that have a balance", () => {
    expect(netWorthFromAccounts([account("Giro", "EUR", 150000), account("Savings", "eur", 1000050)])).toEqual({
      totalMinor: 1150050,
      included: 2,
      excluded: [],
    })
  })

  it("counts negative balances", () => {
    expect(netWorthFromAccounts([account("Giro", "EUR", 50000), account("Credit card", "EUR", -120000)]).totalMinor).toBe(-70000)
  })

  it("leaves out other currencies and accounts without a balance, with the reason", () => {
    expect(
      netWorthFromAccounts([
        account("Giro", "EUR", 150000),
        account("Dollars", "USD", 999999),
        account("Cash", "EUR"),
        account("Francs", "CHF"),
      ]),
    ).toEqual({
      totalMinor: 150000,
      included: 1,
      excluded: [
        { id: "acc-Dollars", name: "Dollars", reason: "currency" },
        { id: "acc-Cash", name: "Cash", reason: "no_balance" },
        { id: "acc-Francs", name: "Francs", reason: "currency" },
      ],
    })
  })

  it("is empty without accounts", () => {
    expect(netWorthFromAccounts([])).toEqual({ totalMinor: 0, included: 0, excluded: [] })
  })
})
