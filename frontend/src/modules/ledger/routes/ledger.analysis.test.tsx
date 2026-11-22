import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"
import { stubChartSize } from "@/test/chart-size"
import { renderWithProviders } from "@/test/render"
import type { LedgerCashFlowAnalysis, LedgerCategory, LedgerRankingAnalysis, LedgerSpendingAnalysis } from "@/modules/ledger/types"
import { LedgerAnalysisPage } from "@/modules/ledger/routes/ledger.analysis"

const at = "2026-01-01T10:00:00Z"

function category(id: string, name: string, parentId?: string): LedgerCategory {
  return { id, name, parentId, matchWords: [], createdAt: at, updatedAt: at }
}

const categories = [category("cat-food", "Food"), category("cat-groceries", "Groceries", "cat-food"), category("cat-car", "Car")]

const topLevel: LedgerSpendingAnalysis = {
  from: "2025-12-01",
  to: "2026-11-22",
  months: ["2026-10", "2026-11"],
  series: [
    { kind: "category", categoryId: "cat-food", name: "Food", amountsMinor: [40000, 35000], totalMinor: 75000 },
    { kind: "category", categoryId: "cat-car", name: "Car", amountsMinor: [0, 12000], totalMinor: 12000 },
    { kind: "uncategorized", name: "", amountsMinor: [500, 0], totalMinor: 500 },
  ],
  totalsMinor: [40500, 47000],
  excludedCurrencies: [],
}

const subcategories: LedgerSpendingAnalysis = {
  ...topLevel,
  parentCategoryId: "cat-food",
  series: [
    { kind: "category", categoryId: "cat-groceries", name: "Groceries", amountsMinor: [30000, 36000], totalMinor: 66000 },
    { kind: "direct", categoryId: "cat-food", name: "", amountsMinor: [10000, -1000], totalMinor: 9000 },
  ],
}

const empty: LedgerSpendingAnalysis = { ...topLevel, series: [], totalsMinor: [0, 0] }

const cashFlow: LedgerCashFlowAnalysis = {
  from: "2024-03-05",
  to: "2026-11-22",
  months: ["2026-10", "2026-11"],
  incomeMinor: [300000, 310000],
  expensesMinor: [40500, 47000],
  netMinor: [259500, 263000],
  excludedCurrencies: [],
}

const noRanking: LedgerRankingAnalysis = { from: "2025-12-01", to: "2026-11-22", categories: [], counterparties: [], excludedCurrencies: [] }

const ranking: LedgerRankingAnalysis = {
  ...noRanking,
  categories: [
    { kind: "category", categoryId: "cat-rent", name: "Rent", amountMinor: 180000 },
    { kind: "uncategorized", name: "", amountMinor: 45000 },
  ],
  counterparties: [
    { name: "Landlord", amountMinor: 180000 },
    { name: "", unknown: true, amountMinor: 9000 },
    { name: "Supermarkt Nord", amountMinor: 4550 },
  ],
}

const base = "/ledger/analysis/"

// A handler is a response body or a function of the call. The ranking is
// empty by default so its names do not mix with the chart legends.
function setup(handlers: { spending?: unknown; cashFlow?: unknown; ranking?: unknown } = {}) {
  const api = mockApi({
    "GET /ledger/categories": categories,
    [`GET ${base}spending`]: handlers.spending ?? ((call: ApiCall) => (call.path.includes("parentCategoryId") ? subcategories : topLevel)),
    [`GET ${base}cash-flow`]: handlers.cashFlow ?? cashFlow,
    [`GET ${base}ranking`]: handlers.ranking ?? noRanking,
  })
  const queries = (endpoint: string) =>
    api.calls.filter((c) => c.path.startsWith(base + endpoint)).map((c) => c.path.slice(base.length + endpoint.length))
  return { api, queries, spendingCalls: () => queries("spending"), ...renderWithProviders(<LedgerAnalysisPage />) }
}

function card(title: string): HTMLElement {
  const found = screen.getByText(title).closest<HTMLElement>('[data-slot="card"]')
  if (!found) throw new Error(`no card titled ${title}`)
  return found
}

const legendOf = (title: string) =>
  [...card(title).querySelectorAll(".recharts-responsive-container + div > div")].map((item) => item.textContent)
const legend = () => legendOf("Expenses by category")
const listOf = (title: string) => within(card(title)).queryAllByRole("listitem").map((item) => item.textContent)

describe("LedgerAnalysisPage", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 10, 22, 9, 0, 0))
    stubChartSize()
  })

  it("loads the last 12 months and shows the series of the response", async () => {
    const { spendingCalls } = setup()

    expect(await screen.findByText("Food")).toBeInTheDocument()
    expect(spendingCalls()).toEqual(["?from=2025-12-01&to=2026-11-22"])
    expect(legend()).toEqual(["Food", "Car", "Uncategorized"])
    expect(screen.getByRole("button", { name: "12 months" })).toHaveAttribute("aria-pressed", "true")
    expect(screen.getByRole("button", { name: "6 months" })).toHaveAttribute("aria-pressed", "false")
    expect(screen.getByText("Dec 1, 2025 – Nov 22, 2026")).toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Analysis" })).toBeInTheDocument()
    // Food in two months, Car and Uncategorized in one each
    await waitFor(() => expect(card("Expenses by category").querySelectorAll(".recharts-bar-rectangle path").length).toBe(4))
  })

  it("refetches with the dates of a preset, and without dates for All", async () => {
    const { user, spendingCalls, queries } = setup()
    await screen.findByText("Food")

    await user.click(screen.getByRole("button", { name: "6 months" }))
    await waitFor(() => expect(spendingCalls()).toContain("?from=2026-06-01&to=2026-11-22"))
    expect(screen.getByText("Jun 1, 2026 – Nov 22, 2026")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Year to date" }))
    await waitFor(() => expect(spendingCalls()).toContain("?from=2026-01-01&to=2026-11-22"))

    await user.click(screen.getByRole("button", { name: "All" }))
    await waitFor(() => expect(spendingCalls()).toContain(""))
    // the effective dates of "all" come from the cash-flow response
    expect(await screen.findByText("All time (Mar 5, 2024 – Nov 22, 2026)")).toBeInTheDocument()
    expect(spendingCalls()).toHaveLength(4)
    // every analysis follows the range
    const expected = ["?from=2025-12-01&to=2026-11-22", "?from=2026-06-01&to=2026-11-22", "?from=2026-01-01&to=2026-11-22", ""]
    for (const endpoint of ["spending", "cash-flow", "ranking"]) {
      expect(queries(endpoint)).toEqual(expected)
    }
  })

  it("sends custom mid-month dates unchanged", async () => {
    const { user, spendingCalls } = setup()
    await screen.findByText("Food")

    await user.click(screen.getByRole("button", { name: "Custom" }))
    // prefilled with the active range, so nothing is fetched yet
    expect(screen.getByLabelText("From")).toHaveValue("2025-12-01")
    expect(screen.getByLabelText("To")).toHaveValue("2026-11-22")
    expect(screen.getByLabelText("To")).toHaveAttribute("max", "2026-11-22")
    expect(spendingCalls()).toHaveLength(1)

    await user.clear(screen.getByLabelText("From"))
    await user.type(screen.getByLabelText("From"), "2026-03-17")
    await user.clear(screen.getByLabelText("To"))
    await user.type(screen.getByLabelText("To"), "2026-04-09")

    await waitFor(() => expect(spendingCalls()).toContain("?from=2026-03-17&to=2026-04-09"))
    expect(screen.getByText("Mar 17, 2026 – Apr 9, 2026")).toBeInTheDocument()
    expect(screen.queryByRole("alert")).toBeNull()
  })

  it("shows why a custom range is invalid and sends nothing", async () => {
    const { user, spendingCalls } = setup()
    await screen.findByText("Food")
    await user.click(screen.getByRole("button", { name: "Custom" }))

    await user.clear(screen.getByLabelText("From"))
    expect(await screen.findByText("Enter both dates.")).toBeInTheDocument()

    await user.type(screen.getByLabelText("From"), "2026-11-23")
    expect(await screen.findByText("The start date must not be after the end date.")).toBeInTheDocument()

    await user.clear(screen.getByLabelText("From"))
    await user.type(screen.getByLabelText("From"), "2026-11-01")
    await user.clear(screen.getByLabelText("To"))
    await user.type(screen.getByLabelText("To"), "2026-12-24")
    expect(await screen.findByText("The end date must not be in the future.")).toBeInTheDocument()

    // the last valid range stays active and its data stays on screen
    expect(spendingCalls().filter((query) => query.includes("2026-11-23") || query.includes("2026-12-24"))).toEqual([])
    expect(screen.getByText("Food")).toBeInTheDocument()
  })

  it("sends nothing for the dates a date input reports while a year is typed", async () => {
    const { user, spendingCalls } = setup()
    await screen.findByText("Food")
    await user.click(screen.getByRole("button", { name: "Custom" }))

    for (const typing of ["0002-12-01", "0020-12-01", "0202-12-01"]) {
      await user.clear(screen.getByLabelText("From"))
      await user.type(screen.getByLabelText("From"), typing)
      expect(screen.getByLabelText("From")).toHaveValue(typing)
      expect(await screen.findByText("The range must not span more than 100 years.")).toBeInTheDocument()
    }
    expect(spendingCalls()).toEqual(["?from=2025-12-01&to=2026-11-22"])

    await user.clear(screen.getByLabelText("From"))
    await user.type(screen.getByLabelText("From"), "2025-11-01")
    await waitFor(() => expect(spendingCalls()).toEqual(["?from=2025-12-01&to=2026-11-22", "?from=2025-11-01&to=2026-11-22"]))
    expect(screen.queryByRole("alert")).toBeNull()
  })

  it("shows the subcategories of a chosen category and goes back", async () => {
    const { user, spendingCalls } = setup()
    await screen.findByText("Food")
    expect(screen.queryByRole("button", { name: "All categories" })).toBeNull()

    await user.click(screen.getByRole("combobox", { name: "Category level" }))
    // only top-level categories that have subcategories are offered
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["Top-level categories", "Food"])
    await user.click(screen.getByRole("option", { name: "Food" }))

    await waitFor(() => expect(legend()).toEqual(["Groceries", "Food (directly)"]))
    expect(spendingCalls()).toContain("?from=2025-12-01&to=2026-11-22&parentCategoryId=cat-food")

    await user.click(screen.getByRole("button", { name: "All categories" }))
    await waitFor(() => expect(legend()).toEqual(["Food", "Car", "Uncategorized"]))
    expect(screen.queryByRole("button", { name: "All categories" })).toBeNull()
  })

  it("shows the empty state", async () => {
    setup({ spending: empty })
    expect(await screen.findByText("No expenses in this period.")).toBeInTheDocument()
    expect(legend()).toEqual([])
  })

  it("shows a placeholder while loading", async () => {
    let respond: (value: LedgerSpendingAnalysis) => void = () => {}
    const response = new Promise<LedgerSpendingAnalysis>((resolve) => {
      respond = resolve
    })
    setup({ spending: () => response })

    await screen.findByText("Income")
    expect(within(card("Expenses by category")).getByRole("status", { name: "Loading..." })).toBeInTheDocument()
    respond(topLevel)
    expect(await screen.findByText("Car")).toBeInTheDocument()
    expect(screen.queryByRole("status", { name: "Loading..." })).toBeNull()
  })

  it("shows an error with a retry", async () => {
    let failing = true
    const { user, spendingCalls } = setup({ spending: () => (failing ? apiError(500, "internal error") : topLevel) })

    expect(await screen.findByText("This analysis could not be loaded.")).toBeInTheDocument()
    failing = false
    await user.click(screen.getByRole("button", { name: "Retry" }))

    expect(await screen.findByText("Food")).toBeInTheDocument()
    expect(screen.queryByText("This analysis could not be loaded.")).toBeNull()
    expect(spendingCalls()).toHaveLength(2)
  })

  it("starts the custom dates of All with the dates the data covers", async () => {
    const { user, queries } = setup()
    await screen.findByText("Food")
    await user.click(screen.getByRole("button", { name: "All" }))
    await screen.findByText("All time (Mar 5, 2024 – Nov 22, 2026)")

    await user.click(screen.getByRole("button", { name: "Custom" }))
    expect(screen.getByLabelText("From")).toHaveValue("2024-03-05")
    expect(screen.getByLabelText("To")).toHaveValue("2026-11-22")
    expect(screen.queryByRole("alert")).toBeNull()
    await waitFor(() => expect(queries("cash-flow")).toContain("?from=2024-03-05&to=2026-11-22"))
  })

  it("shows the cash flow with its legend", async () => {
    const { queries } = setup()

    await waitFor(() => expect(legendOf("Cash flow")).toEqual(["Income", "Expenses", "Net result"]))
    expect(queries("cash-flow")).toEqual(["?from=2025-12-01&to=2026-11-22"])
    // income and expenses of two months as bars, the net result as a line
    await waitFor(() => expect(card("Cash flow").querySelectorAll(".recharts-bar-rectangle path").length).toBe(4))
    expect(card("Cash flow").querySelectorAll(".recharts-line-curve").length).toBe(1)
  })

  it("shows the cash-flow empty state and error with retry on their own", async () => {
    let failing = false
    const zero = { ...cashFlow, incomeMinor: [0, 0], expensesMinor: [0, 0], netMinor: [0, 0] }
    const { user, queryClient } = setup({ cashFlow: () => (failing ? apiError(500, "internal error") : zero) })

    expect(await screen.findByText("No income or expenses in this period.")).toBeInTheDocument()
    expect(await screen.findByText("Food")).toBeInTheDocument()

    failing = true
    await queryClient.refetchQueries({ queryKey: ["ledger", "analysis", "cash-flow"] })
    const cashFlowCard = within(card("Cash flow"))
    expect(await cashFlowCard.findByText("This analysis could not be loaded.")).toBeInTheDocument()
    expect(screen.getAllByText("This analysis could not be loaded.")).toHaveLength(1)
    expect(legend()).toEqual(["Food", "Car", "Uncategorized"])

    failing = false
    await user.click(cashFlowCard.getByRole("button", { name: "Retry" }))
    expect(await screen.findByText("No income or expenses in this period.")).toBeInTheDocument()
  })

  it("lists the top categories and counterparties in order", async () => {
    setup({ ranking })

    await waitFor(() => expect(listOf("Top categories")).toEqual(["Rent€1,800.00", "Uncategorized€450.00"]))
    expect(listOf("Top counterparties")).toEqual(["Landlord€1,800.00", "Unknown counterparty€90.00", "Supermarkt Nord€45.50"])
  })

  it("shows an empty line per list and an error for both", async () => {
    let failing = false
    const { queryClient } = setup({
      ranking: () => (failing ? apiError(500, "internal error") : { ...ranking, counterparties: [] }),
    })

    await waitFor(() => expect(listOf("Top categories")).toHaveLength(2))
    expect(within(card("Top counterparties")).getByText("No expenses in this period.")).toBeInTheDocument()
    expect(within(card("Top categories")).queryByText("No expenses in this period.")).toBeNull()

    failing = true
    await queryClient.refetchQueries({ queryKey: ["ledger", "analysis", "ranking"] })
    await waitFor(() => expect(screen.getAllByText("This analysis could not be loaded.")).toHaveLength(2))
    expect(within(card("Top categories")).getByRole("button", { name: "Retry" })).toBeInTheDocument()
  })

  it("names the currencies that were left out, only when there are any", async () => {
    const { unmount } = setup()
    await waitFor(() => expect(legendOf("Cash flow")).toHaveLength(3))
    expect(screen.queryByText(/are not included/)).toBeNull()
    unmount()

    setup({ cashFlow: { ...cashFlow, excludedCurrencies: ["CHF", "USD"] } })
    expect(await screen.findByText("Transactions in CHF, USD are not included.")).toBeInTheDocument()
  })
})
