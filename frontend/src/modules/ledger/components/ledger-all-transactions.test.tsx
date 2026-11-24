import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount, LedgerTransaction } from "@/modules/ledger/types"
import { LedgerAllTransactions } from "@/modules/ledger/components/ledger-all-transactions"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}))

const at = "2026-11-01T10:00:00Z"

function account(id: string, name: string): LedgerAccount {
  return { id, name, bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at } as LedgerAccount
}

function txn(id: string, accountId: string, bookingDate: string): LedgerTransaction {
  return {
    id, accountId, bookingDate, amountMinor: -1000, currency: "EUR", counterpartyName: `Shop ${id}`,
    reviewStatus: "confirmed", categorizationSource: "manual", sourceType: "dkb.csv",
    importBatchId: "batch", fingerprint: `fp-${id}`, createdAt: at, updatedAt: at,
  } as LedgerTransaction
}

const giro = account("acc-giro", "Giro")
const savings = account("acc-savings", "Savings")

function listCalls(calls: ApiCall[]) {
  return calls.filter((call) => call.path.startsWith("/ledger/transactions")).map((call) => call.path)
}

function setup(accountIds?: string[]) {
  const onAccountIdsChange = vi.fn()
  const api = mockApi({
    "GET /ledger/accounts": [giro, savings],
    "GET /ledger/categories": [],
    "GET /ledger/transactions": (call: ApiCall) => {
      const params = new URL(call.path, "http://x").searchParams
      if (params.get("cursor") === "b") return { items: [txn("c", "acc-giro", "2026-10-01")], total: 3 }
      return { items: [txn("a", "acc-giro", "2026-11-03"), txn("b", "acc-savings", "2026-11-02")], nextCursor: "b", total: 3 }
    },
  })
  const view = renderWithProviders(<LedgerAllTransactions accountIds={accountIds} onAccountIdsChange={onAccountIdsChange} />)
  return { api, onAccountIdsChange, ...view }
}

describe("LedgerAllTransactions", () => {
  it("lists the transactions of all accounts with their account and appends the next page", async () => {
    const { api, user } = setup()

    const first = await screen.findByText("Shop a")
    expect(screen.getByText("2 of 3 transactions shown")).toBeInTheDocument()
    expect(within(first.closest("tr") as HTMLElement).getByText("Giro")).toBeInTheDocument()
    expect(within(screen.getByText("Shop b").closest("tr") as HTMLElement).getByText("Savings")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Load more" }))
    await screen.findByText("Shop c")
    expect(screen.getByText("Shop a")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull()
    expect(screen.getByText("3 of 3 transactions shown")).toBeInTheDocument()
    expect(listCalls(api.calls)).toEqual(["/ledger/transactions?limit=100", "/ledger/transactions?limit=100&cursor=b"])
  })

  it("sends the selected accounts and ignores unknown ones", async () => {
    const { api } = setup(["acc-savings", "acc-gone"])

    await screen.findByText("Shop a")
    expect(listCalls(api.calls)).toEqual(["/ledger/transactions?limit=100&accountId=acc-savings"])
    expect(screen.getByRole("button", { name: "Filter accounts: Savings" })).toBeInTheDocument()
  })

  it("changes the selection through the filter", async () => {
    const { onAccountIdsChange, user } = setup(["acc-giro"])

    await screen.findByText("Shop a")
    await user.click(screen.getByRole("button", { name: "Filter accounts: Giro" }))
    await user.click(await screen.findByRole("menuitemcheckbox", { name: "Savings" }))
    expect(onAccountIdsChange).toHaveBeenLastCalledWith(["acc-giro", "acc-savings"])

    await user.click(screen.getByRole("menuitem", { name: "Show all accounts" }))
    await waitFor(() => expect(onAccountIdsChange).toHaveBeenLastCalledWith([]))
  })
})
