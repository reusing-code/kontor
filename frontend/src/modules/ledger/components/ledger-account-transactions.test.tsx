import { useState, type ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerTransaction } from "@/modules/ledger/types"
import { LedgerAccountTransactions } from "@/modules/ledger/components/ledger-account-transactions"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children }: { children: ReactNode }) => <a href="/">{children}</a>,
}))

const at = "2026-11-01T10:00:00Z"
const current = 1_000_000

// 250 transactions of -1.00 EUR each, newest first: "Payee 1" … "Payee 250".
const transactions: LedgerTransaction[] = Array.from({ length: 250 }, (_, i) => ({
  id: `txn-${i + 1}`,
  accountId: "acc",
  bookingDate: "2026-11-01",
  amountMinor: -100,
  currency: "EUR",
  counterpartyName: `Payee ${i + 1}`,
  reviewStatus: "confirmed",
  categorizationSource: "manual",
  sourceType: "dkb.csv",
  importBatchId: "batch",
  fingerprint: `fp-${i + 1}`,
  createdAt: at,
  updatedAt: at,
}))

function accountPage(call: ApiCall) {
  const params = new URLSearchParams(call.path.split("?")[1])
  const limit = Number(params.get("limit"))
  const offset = Math.min(Number(params.get("offset")), transactions.length)
  const items = transactions.slice(offset, offset + limit)
  const closingMinor = current + offset * 100
  return { items, offset, total: transactions.length, pageBalance: { closingMinor, openingMinor: closingMinor + items.length * 100 } }
}

function Harness({ initialPage, onPageChange }: { initialPage: number; onPageChange?: (page: number, options?: { replace?: boolean }) => void }) {
  const [page, setPage] = useState(initialPage)
  return (
    <LedgerAccountTransactions
      accountId="acc"
      categories={[]}
      page={page}
      onPageChange={(next, options) => {
        onPageChange?.(next, options)
        setPage(next)
      }}
    />
  )
}

function setup(initialPage = 1, onPageChange?: (page: number, options?: { replace?: boolean }) => void) {
  const api = mockApi({ "GET /ledger/accounts/acc/transactions": accountPage })
  return { api, ...renderWithProviders(<Harness initialPage={initialPage} onPageChange={onPageChange} />) }
}

const requests = (api: ReturnType<typeof mockApi>) => api.calls.map((call) => call.path)
const balanceRow = (label: string) => screen.getByText(label).closest("tr")!

describe("LedgerAccountTransactions", () => {
  it("shows one page at a time and jumps between pages", async () => {
    const { api, user } = setup()

    expect(await screen.findByText("Payee 1")).toBeInTheDocument()
    expect(screen.getByText("Payee 100")).toBeInTheDocument()
    expect(screen.queryByText("Payee 101")).toBeNull()
    expect(screen.getByText("1–100 of 250 transactions")).toBeInTheDocument()
    expect(screen.getAllByRole("navigation", { name: "Transaction pages" })).toHaveLength(2)
    expect(screen.queryByRole("button", { name: /load more/i })).toBeNull()

    const [top] = screen.getAllByRole("navigation", { name: "Transaction pages" })
    await user.click(within(top).getByRole("button", { name: "Page 3" }))
    expect(await screen.findByText("Payee 201")).toBeInTheDocument()
    expect(screen.getByText("Payee 250")).toBeInTheDocument()
    expect(screen.queryByText("Payee 1")).toBeNull()
    expect(screen.getByText("201–250 of 250 transactions")).toBeInTheDocument()
    expect(within(top).getByRole("button", { name: "Page 3" })).toHaveAttribute("aria-current", "page")

    await user.click(within(top).getByRole("button", { name: "Previous page" }))
    expect(await screen.findByText("Payee 101")).toBeInTheDocument()
    expect(screen.queryByText("Payee 201")).toBeNull()

    expect(requests(api)).toEqual([
      "/ledger/accounts/acc/transactions?limit=100&offset=0",
      "/ledger/accounts/acc/transactions?limit=100&offset=200",
      "/ledger/accounts/acc/transactions?limit=100&offset=100",
    ])

    // a page seen before shows right away from the cache
    await user.click(within(top).getByRole("button", { name: "Page 1" }))
    expect(screen.getByText("Payee 1")).toBeInTheDocument()
  })

  it("shows the balances around the shown page", async () => {
    const { user } = setup()

    await screen.findByText("Payee 1")
    expect(balanceRow("Balance after these transactions")).toHaveTextContent("10,000.00")
    expect(balanceRow("Balance before these transactions")).toHaveTextContent("10,100.00")

    await user.click(screen.getAllByRole("button", { name: "Page 2" })[1])
    await screen.findByText("Payee 101")
    expect(balanceRow("Balance after these transactions")).toHaveTextContent("10,100.00")
    expect(balanceRow("Balance before these transactions")).toHaveTextContent("10,200.00")
  })

  it("moves a page past the end to the last page", async () => {
    const onPageChange = vi.fn()
    const { api } = setup(9, onPageChange)

    expect(await screen.findByText("Payee 201")).toBeInTheDocument()
    expect(onPageChange).toHaveBeenCalledWith(3, { replace: true })
    await waitFor(() => expect(requests(api).at(-1)).toBe("/ledger/accounts/acc/transactions?limit=100&offset=200"))
  })
})
