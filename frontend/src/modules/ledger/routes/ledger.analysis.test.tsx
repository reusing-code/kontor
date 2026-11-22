import { beforeEach, describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"
import { stubChartSize } from "@/test/chart-size"
import { renderWithProviders } from "@/test/render"
import type { LedgerCategory, LedgerSpendingAnalysis } from "@/modules/ledger/types"
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

const spendingPath = "/ledger/analysis/spending"

function setup(spending: unknown = (call: ApiCall) => (call.path.includes("parentCategoryId") ? subcategories : topLevel)) {
  const api = mockApi({
    "GET /ledger/categories": categories,
    [`GET ${spendingPath}`]: spending,
  })
  const spendingCalls = () => api.calls.filter((c) => c.path.startsWith(spendingPath)).map((c) => c.path.slice(spendingPath.length))
  return { api, spendingCalls, ...renderWithProviders(<LedgerAnalysisPage />) }
}

const legend = () => [...document.querySelectorAll(".recharts-responsive-container + div > div")].map((item) => item.textContent)

describe("LedgerAnalysisPage", () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 10, 22, 9, 0, 0))
    stubChartSize()
  })

  it("loads the last 12 months and shows the series of the response", async () => {
    const { spendingCalls, container } = setup()

    expect(await screen.findByText("Food")).toBeInTheDocument()
    expect(spendingCalls()).toEqual(["?from=2025-12-01&to=2026-11-22"])
    expect(legend()).toEqual(["Food", "Car", "Uncategorized"])
    expect(screen.getByRole("button", { name: "12 months" })).toHaveAttribute("aria-pressed", "true")
    expect(screen.getByRole("button", { name: "6 months" })).toHaveAttribute("aria-pressed", "false")
    expect(screen.getByText("Dec 1, 2025 – Nov 22, 2026")).toBeInTheDocument()
    expect(screen.getByRole("heading", { name: "Analysis" })).toBeInTheDocument()
    // Food in two months, Car and Uncategorized in one each
    await waitFor(() => expect(container.querySelectorAll(".recharts-bar-rectangle path").length).toBe(4))
  })

  it("refetches with the dates of a preset, and without dates for All", async () => {
    const { user, spendingCalls } = setup()
    await screen.findByText("Food")

    await user.click(screen.getByRole("button", { name: "6 months" }))
    await waitFor(() => expect(spendingCalls()).toContain("?from=2026-06-01&to=2026-11-22"))
    expect(screen.getByText("Jun 1, 2026 – Nov 22, 2026")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Year to date" }))
    await waitFor(() => expect(spendingCalls()).toContain("?from=2026-01-01&to=2026-11-22"))

    await user.click(screen.getByRole("button", { name: "All" }))
    await waitFor(() => expect(spendingCalls()).toContain(""))
    expect(screen.getByText("All time")).toBeInTheDocument()
    expect(spendingCalls()).toHaveLength(4)
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
    setup(empty)
    expect(await screen.findByText("No expenses in this period.")).toBeInTheDocument()
    expect(legend()).toEqual([])
  })

  it("shows a placeholder while loading", async () => {
    let respond: (value: LedgerSpendingAnalysis) => void = () => {}
    const response = new Promise<LedgerSpendingAnalysis>((resolve) => {
      respond = resolve
    })
    setup(() => response)

    expect(await screen.findByRole("status", { name: "Loading..." })).toBeInTheDocument()
    respond(topLevel)
    expect(await screen.findByText("Car")).toBeInTheDocument()
    expect(screen.queryByRole("status", { name: "Loading..." })).toBeNull()
  })

  it("shows an error with a retry", async () => {
    let failing = true
    const { user, spendingCalls } = setup(() => (failing ? apiError(500, "internal error") : topLevel))

    expect(await screen.findByText("This analysis could not be loaded.")).toBeInTheDocument()
    failing = false
    await user.click(screen.getByRole("button", { name: "Retry" }))

    expect(await screen.findByText("Food")).toBeInTheDocument()
    expect(screen.queryByText("This analysis could not be loaded.")).toBeNull()
    expect(spendingCalls()).toHaveLength(2)
  })
})
