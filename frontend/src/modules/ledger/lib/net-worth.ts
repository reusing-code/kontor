import type { LedgerAccount } from "@/modules/ledger/types"

// This module stays free of chart imports: the ledger dashboard uses it.

export type NetWorthExclusion = "no_balance" | "currency"

export interface AccountsNetWorth {
  totalMinor: number
  included: number
  excluded: Array<{ id: string; name: string; reason: NetWorthExclusion }>
}

// netWorthFromAccounts sums the current balances by the rule of the net-worth
// analysis: EUR accounts with a balance count, the others are listed with the
// reason (another currency wins over a missing balance).
export function netWorthFromAccounts(accounts: LedgerAccount[]): AccountsNetWorth {
  const result: AccountsNetWorth = { totalMinor: 0, included: 0, excluded: [] }
  for (const account of accounts) {
    const currency = account.currency.trim().toUpperCase()
    if (currency !== "" && currency !== "EUR") {
      result.excluded.push({ id: account.id, name: account.name, reason: "currency" })
    } else if (!account.balance) {
      result.excluded.push({ id: account.id, name: account.name, reason: "no_balance" })
    } else {
      result.totalMinor += account.balance.currentMinor
      result.included += 1
    }
  }
  return result
}
