import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { stubChartSize } from "@/test/chart-size"
import { renderWithProviders } from "@/test/render"
import { LedgerAnalysisPage } from "@/modules/ledger/routes/ledger.analysis"

const base = "/ledger/analysis/"
const range = { from: "2025-12-01", to: "2026-11-22" }
const months = ["2026-10", "2026-11"]

const spending = { ...range, months, series: [], totalsMinor: [0, 0], excludedCurrencies: [] }
const cashFlow = { ...range, months, incomeMinor: [0, 0], expensesMinor: [0, 0], netMinor: [0, 0], excludedCurrencies: [] }
const ranking = { ...range, categories: [], counterparties: [], excludedCurrencies: [] }
const netWorth = { ...range, dates: [], totalMinor: [], accounts: [], excludedAccounts: [] }
const breakdown = { ...range, totalMinor: 0, categories: [], droppedInflows: 0, excludedCurrencies: [] }

function setup(handlers: { spending?: unknown; cashFlow?: unknown; ranking?: unknown } = {}) {
  const api = mockApi({
    "GET /ledger/categories": [],
    [`GET ${base}spending`]: handlers.spending ?? spending,
    [`GET ${base}cash-flow`]: handlers.cashFlow ?? cashFlow,
    [`GET ${base}ranking`]: handlers.ranking ?? ranking,
    [`GET ${base}net-worth`]: netWorth,
    [`GET ${base}category-breakdown`]: breakdown,
  })
  const queries = (endpoint: string) =>
    api.calls.filter((c) => c.path.startsWith(base + endpoint)).map((c) => c.path.slice(base.length + endpoint.length))
  return { api, queries, ...renderWithProviders(<LedgerAnalysisPage />) }
}

function card(title: string): HTMLElement {
  const found = screen.getByText(title).closest<HTMLElement>('[data-slot="card"]')
  if (!found) throw new Error(`no card titled ${title}`)
  return found
}

describe("LedgerAnalysisPage with unusual data", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 10, 22, 9, 0, 0))
    stubChartSize()
  })

  it("shows names from the ledger as text, never as markup", async () => {
    const hostile = '<img src=x onerror="window.__pwned = true">'
    const { container } = setup({
      spending: {
        ...spending,
        series: [{ kind: "category", categoryId: "cat-x", name: "<b>Food</b>", amountsMinor: [100, 200], totalMinor: 300 }],
        totalsMinor: [100, 200],
      },
      ranking: {
        ...ranking,
        categories: [{ kind: "category", categoryId: "cat-x", name: "<b>Food</b>", amountMinor: 300 }],
        counterparties: [{ name: hostile, amountMinor: 300 }],
      },
    })

    expect(await within(card("Top counterparties")).findByText(hostile)).toBeInTheDocument()
    expect(within(card("Top categories")).getByText("<b>Food</b>")).toBeInTheDocument()
    await waitFor(() => expect(within(card("Expenses by category")).getByText("<b>Food</b>")).toBeInTheDocument())
    expect(container.querySelector("img, b")).toBeNull()
    expect((window as { __pwned?: boolean }).__pwned).toBeUndefined()
  })

  it("ranks ten entries and keeps every bar inside its list", async () => {
    const counterparties = Array.from({ length: 10 }, (_, index) => ({ name: `Shop ${index}`, amountMinor: 100_000 - index * 11_111 }))
    setup({ ranking: { ...ranking, counterparties: [...counterparties.slice(0, 9), { name: "Tiny", amountMinor: 1 }] } })

    const list = within(card("Top counterparties"))
    await waitFor(() => expect(list.getAllByRole("listitem")).toHaveLength(10))
    const widths = list.getAllByRole("listitem").map((item) => Number.parseFloat(item.querySelector<HTMLElement>("div.rounded-full")!.style.width))
    expect(widths[0]).toBe(100)
    expect(Math.max(...widths)).toBeLessThanOrEqual(100)
    // an entry far below the first one still shows a sliver
    expect(widths[9]).toBe(1)
    expect([...widths].sort((a, b) => b - a)).toEqual(widths)
  })

  it("shows All on a ledger without bookings as today's date and empty analyses", async () => {
    const today = { from: "2026-11-22", to: "2026-11-22" }
    const { user, queries } = setup({
      spending: { ...spending, ...today, months: ["2026-11"], totalsMinor: [0] },
      cashFlow: { ...cashFlow, ...today, months: ["2026-11"], incomeMinor: [0], expensesMinor: [0], netMinor: [0] },
      ranking: { ...ranking, ...today },
    })
    await screen.findByText("No income or expenses in this period.")

    await user.click(screen.getByRole("button", { name: "All" }))
    expect(await screen.findByText("All time (Nov 22, 2026 – Nov 22, 2026)")).toBeInTheDocument()
    expect(screen.queryByRole("alert")).toBeNull()

    await user.click(screen.getByRole("button", { name: "Custom" }))
    expect(screen.getByLabelText("From")).toHaveValue("2026-11-22")
    expect(screen.getByLabelText("To")).toHaveValue("2026-11-22")
    expect(screen.queryByRole("alert")).toBeNull()
    await waitFor(() => expect(queries("cash-flow")).toContain("?from=2026-11-22&to=2026-11-22"))
  })

  // The server's UTC date can be a day ahead of the browser's. "All" on a
  // ledger whose first booking is that day must not prefill "Custom" with a
  // start after the capped end.
  it("opens Custom after All without an error when the server is a day ahead", async () => {
    const tomorrow = { from: "2026-11-23", to: "2026-11-23" }
    const { user } = setup({ cashFlow: { ...cashFlow, ...tomorrow, months: ["2026-11"], incomeMinor: [0], expensesMinor: [0], netMinor: [0] } })
    await screen.findByText("No income or expenses in this period.")
    await user.click(screen.getByRole("button", { name: "All" }))
    await screen.findByText("All time (Nov 23, 2026 – Nov 23, 2026)")

    await user.click(screen.getByRole("button", { name: "Custom" }))
    expect(screen.getByLabelText("From")).toHaveValue("2026-11-22")
    expect(screen.getByLabelText("To")).toHaveValue("2026-11-22")
    expect(screen.queryByRole("alert")).toBeNull()
  })
})
