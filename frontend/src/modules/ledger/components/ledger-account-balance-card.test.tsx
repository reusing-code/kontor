import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount } from "@/modules/ledger/types"
import { todayISO } from "@/modules/ledger/lib/balance"
import { LedgerAccountBalanceCard } from "@/modules/ledger/components/ledger-account-balance-card"

const at = "2026-11-01T10:00:00Z"
const account: LedgerAccount = { id: "acc-1", name: "Giro", bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at }

describe("LedgerAccountBalanceCard", () => {
  it("shows the current balance, where it comes from and the drift", () => {
    mockApi({})
    renderWithProviders(
      <LedgerAccountBalanceCard
        account={{ ...account, balance: { currentMinor: 123456, snapshotMinor: 120000, asOf: "2026-11-02", source: "enablebanking", driftMinor: -500, previousAsOf: "2026-10-01" } }}
      />,
    )
    expect(screen.getByText("€1,234.56")).toBeInTheDocument()
    expect(screen.getByText("Reported for 2026-11-02 · Enable Banking")).toBeInTheDocument()
    expect(screen.getByText("Off by -€5.00")).toBeInTheDocument()
    expect(screen.getByText(/does not match the one for 2026-10-01/)).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Remove the balance set by hand" })).toBeNull()
  })

  it("hides the drift when the balances agree", () => {
    mockApi({})
    renderWithProviders(<LedgerAccountBalanceCard account={{ ...account, balance: { currentMinor: 1, snapshotMinor: 1, asOf: "2026-11-02", source: "dkb.csv", driftMinor: 0 } }} />)
    expect(screen.queryByText(/Off by/)).toBeNull()
  })

  it("sets a balance by hand and refuses an unreadable amount", async () => {
    const api = mockApi({ "POST /ledger/accounts/acc-1/balance": { ...account } })
    const { user } = renderWithProviders(<LedgerAccountBalanceCard account={account} />)
    expect(screen.getByText(/No balance yet/)).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Set balance" }))
    const amount = screen.getByLabelText("Balance")
    await user.type(amount, "1.234,56")
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(screen.getByText("Enter an amount such as 1234,56 or -12,30.")).toBeInTheDocument()
    expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(0)

    await user.clear(amount)
    await user.type(amount, "-12,30")
    expect(screen.getByLabelText("As of")).toHaveValue(todayISO())
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(api.calls.filter((c) => c.method === "POST")).toHaveLength(1))
    expect(api.calls.find((c) => c.method === "POST")?.body).toEqual({ amountMinor: -1230, asOf: todayISO() })
    expect(await screen.findByText("Balance saved.")).toBeInTheDocument()
  })

  it("removes a balance set by hand", async () => {
    const api = mockApi({ "DELETE /ledger/accounts/acc-1/balance/2026-11-02": { ...account } })
    const { user } = renderWithProviders(
      <LedgerAccountBalanceCard account={{ ...account, balance: { currentMinor: 100, snapshotMinor: 100, asOf: "2026-11-02", source: "manual" } }} />,
    )
    expect(screen.getByText("Reported for 2026-11-02 · set by hand")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Remove the balance set by hand" }))
    await waitFor(() => expect(api.calls.some((c) => c.method === "DELETE")).toBe(true))
    expect(await screen.findByText("Balance removed.")).toBeInTheDocument()
  })
})
