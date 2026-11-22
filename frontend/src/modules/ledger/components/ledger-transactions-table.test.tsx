import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import type { LedgerTransaction } from "@/modules/ledger/types"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}))

const at = "2026-11-01T10:00:00Z"

function txn(id: string, bookingDate: string, amountMinor: number): LedgerTransaction {
  return {
    id, accountId: "acc-1", bookingDate, amountMinor, currency: "EUR", counterpartyName: `Shop ${id}`,
    reviewStatus: "confirmed", categorizationSource: "manual", sourceType: "dkb.csv",
    importBatchId: "batch-1", fingerprint: `fp-${id}`, createdAt: at, updatedAt: at,
  } as LedgerTransaction
}

const transactions = [txn("a", "2026-11-03", -2500), txn("b", "2026-11-01", 10000)]

describe("LedgerTransactionsTable balances", () => {
  it("shows the balance after the newest and before the oldest shown transaction", () => {
    renderWithProviders(<LedgerTransactionsTable transactions={transactions} pageBalance={{ closingMinor: 150000, openingMinor: 142500 }} />)

    const rows = screen.getAllByRole("row")
    expect(within(rows[1]).getByText("Balance after these transactions")).toBeInTheDocument()
    expect(within(rows[1]).getByText("€1,500.00")).toBeInTheDocument()
    expect(within(rows[2]).getByText("Shop a")).toBeInTheDocument()
    expect(within(rows[rows.length - 1]).getByText("Balance before these transactions")).toBeInTheDocument()
    expect(within(rows[rows.length - 1]).getByText("€1,425.00")).toBeInTheDocument()
  })

  it("shows no balance rows without a balance or without transactions", () => {
    const { unmount } = renderWithProviders(<LedgerTransactionsTable transactions={transactions} />)
    expect(screen.queryByText("Balance after these transactions")).toBeNull()
    unmount()

    renderWithProviders(<LedgerTransactionsTable transactions={[]} pageBalance={{ closingMinor: 1, openingMinor: 1 }} />)
    expect(screen.queryByText("Balance before these transactions")).toBeNull()
    expect(screen.getByText("No transactions found.", { exact: false })).toBeInTheDocument()
  })
})
