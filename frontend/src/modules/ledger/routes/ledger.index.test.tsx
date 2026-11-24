import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount } from "@/modules/ledger/types"
import { LedgerIndexPage } from "@/modules/ledger/routes/ledger.index"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children, to }: { children: ReactNode; to: string }) => <a href={to}>{children}</a>,
  useNavigate: () => () => undefined,
}))

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

function setup(accounts: LedgerAccount[], reviewTotal = 0) {
  const api = mockApi({
    "GET /settings": { renewalDays: 90, reminderFrequency: "disabled", enabledModules: ["ledger"] },
    "GET /capabilities": { llm: false, llmVision: false, bankImport: false },
    "GET /ledger/accounts": accounts,
    "GET /ledger/categories": [],
    "GET /ledger/email-accounts": [],
    "GET /ledger/email-orders": [],
    "GET /ledger/imports": [],
    "GET /ledger/transactions": { items: [], total: reviewTotal },
  })
  return { api, ...renderWithProviders(<LedgerIndexPage />) }
}

function netWorthCard(): HTMLElement {
  const found = screen.getByText("Net worth").closest<HTMLElement>('[data-slot="card"]')
  if (!found) throw new Error("no net worth card")
  return found
}

describe("LedgerIndexPage net worth card", () => {
  it("sums the balances of the account list without asking for an analysis", async () => {
    const { api } = setup([account("Giro", "EUR", 123456), account("Savings", "eur", -23456), account("Dollars", "USD", 999999), account("Cash", "EUR")])

    const card = within(netWorthCard())
    expect(await card.findByText("€1,000.00")).toBeInTheDocument()
    expect(card.getByRole("link", { name: "Open analysis" })).toHaveAttribute("href", "/ledger/analysis")
    expect(card.getByText("2 accounts are not included.")).toBeInTheDocument()

    await waitFor(() => expect(api.callsTo("GET", "/ledger/imports")).toHaveLength(1))
    expect(api.calls.filter((call) => call.path.startsWith("/ledger/analysis"))).toEqual([])
    expect(api.callsTo("GET", "/ledger/accounts")).toHaveLength(1)
  })

  it("is the first card of the grid and keeps the link without any balance", async () => {
    const { container } = setup([account("Cash", "EUR")])

    const card = netWorthCard()
    await waitFor(() => expect(within(card).queryByText(/not included/)).toBeInTheDocument())
    expect(within(card).getByText("–")).toBeInTheDocument()
    expect(within(card).queryByText(/€/)).toBeNull()
    expect(within(card).getByRole("link", { name: "Open analysis" })).toHaveAttribute("href", "/ledger/analysis")
    expect(container.querySelector('.grid > [data-slot="card"]')).toBe(card)
  })
})

describe("LedgerIndexPage review queue card", () => {
  it("shows the size of the whole queue from one small request", async () => {
    const { api } = setup([], 137)

    const card = within(screen.getByText("Review queue").closest<HTMLElement>('[data-slot="card"]') as HTMLElement)
    expect(await card.findByText("137")).toBeInTheDocument()
    expect(api.calls.filter((call) => call.path.startsWith("/ledger/transactions")).map((call) => call.path)).toEqual(["/ledger/transactions?limit=1"])
  })
})
