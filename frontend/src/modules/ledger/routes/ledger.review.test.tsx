import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"
import type { LedgerBatchReviewItem, LedgerCategory, LedgerReviewQueueItem } from "@/modules/ledger/types"
import { LedgerReviewPage } from "@/modules/ledger/routes/ledger.review"

const at = "2026-11-01T10:00:00Z"
const groceries = "cat-groceries"
const fuel = "cat-fuel"

let seq = 0
function txn(fields: Partial<LedgerReviewQueueItem>): LedgerReviewQueueItem {
  seq += 1
  return {
    id: `txn-${seq}`,
    accountId: "acc",
    bookingDate: "2026-11-01",
    amountMinor: -1000,
    currency: "EUR",
    reviewStatus: "needsReview",
    categorizationSource: "none",
    sourceType: "dkb.csv",
    importBatchId: "batch",
    fingerprint: `fp-${seq}`,
    createdAt: at,
    updatedAt: at,
    ...fields,
  }
}

const aldi = (day: number) =>
  txn({ counterpartyName: `ALDI SAGT DANKE ${day}`, bookingDate: `2026-10-${String(day).padStart(2, "0")}`, categoryId: groceries, categorizationSource: "keyword" })

function category(id: string, name: string, parentId?: string): LedgerCategory {
  return { id, name, parentId, matchWords: [], createdAt: at, updatedAt: at }
}

const defaultCategories = [category(groceries, "Groceries"), category(fuel, "Fuel")]

// failRequest: the number of the review-batch request that fails (1-based).
function setup(initial: LedgerReviewQueueItem[], categories = defaultCategories, failRequest?: number) {
  let queue = initial
  let requests = 0
  const api = mockApi({
    "GET /capabilities": { llm: true, llmVision: false, bankImport: false },
    "GET /ledger/categories": categories,
    "GET /ledger/review-queue": () => ({ items: queue, total: queue.length }),
    "GET /ledger/category-suggestions/status": { running: false },
    "GET /paperless/config": { configured: false },
    "POST /ledger/transactions/review-batch": (call: ApiCall) => {
      if (++requests === failRequest) return apiError(500, "database gone")
      const items = (call.body as { items: LedgerBatchReviewItem[] }).items
      const done = new Set(items.map((item) => item.transactionId))
      queue = queue.filter((item) => !done.has(item.id))
      return { reviewed: items.length, failed: [] }
    },
  })
  return { api, ...renderWithProviders(<LedgerReviewPage />) }
}

function batchBodies(api: ReturnType<typeof mockApi>): LedgerBatchReviewItem[][] {
  return api.callsTo("POST", "/ledger/transactions/review-batch").map((call) => (call.body as { items: LedgerBatchReviewItem[] }).items)
}

const emptyPanel = "Select a transaction from the queue to review it."

describe("LedgerReviewPage", () => {
  it("loads the whole queue in one request", async () => {
    const { api } = setup([txn({ counterpartyName: "Someone" })])
    await screen.findByText("Someone")
    expect(api.calls.filter((call) => call.path.startsWith("/ledger/review-queue")).map((call) => call.path)).toEqual(["/ledger/review-queue?limit=all"])
  })

  it("shows no transaction until one is picked and only lets it go with the close button", async () => {
    const { user } = setup([txn({ counterpartyName: "Someone" })])

    await screen.findByText("Someone")
    expect(screen.getByText(emptyPanel)).toBeInTheDocument()
    await user.click(screen.getByText("Someone"))
    expect(screen.getByText("Review transaction")).toBeInTheDocument()

    await user.click(screen.getAllByText("Someone")[0])
    expect(screen.getByText("Review transaction")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Close" }))
    expect(screen.getByText(emptyPanel)).toBeInTheDocument()
  })

  it("offers the categories as a tree and shows the chosen path", async () => {
    const { user } = setup(
      [txn({ counterpartyName: "Someone" })],
      [category("food", "Food"), category("gro", "Groceries", "food"), category("bak", "Bakery", "food"), category("car", "Car")],
    )

    await user.click(await screen.findByText("Someone"))
    await user.click(screen.getByRole("combobox", { name: "Choose category" }))
    const options = screen.getAllByRole("option")
    expect(options.map((o) => o.textContent)).toEqual(["No category", "Car", "Food", "Bakery", "Groceries"])
    expect(options.map((o) => o.style.paddingLeft)).toEqual(["", "0.5rem", "0.5rem", "1.75rem", "1.75rem"])

    await user.click(screen.getByRole("option", { name: "Groceries" }))
    expect(screen.getByRole("combobox", { name: "Choose category" })).toHaveTextContent("Food › Groceries")
  })

  it("accepts a group with the category chosen on the group row", async () => {
    const items = [aldi(2), aldi(1)]
    const { api, user } = setup(items)

    const select = await screen.findByRole("combobox", { name: "Category for all transactions of ALDI SAGT DANKE 2" })
    expect(select).toHaveTextContent("Groceries")
    await user.click(screen.getByRole("checkbox", { name: "Select all transactions of ALDI SAGT DANKE 2" }))
    expect(screen.getByText("0 of 2 proposed categories selected.")).toBeInTheDocument()

    await user.click(select)
    await user.click(screen.getByRole("option", { name: "Fuel" }))
    expect(select).toHaveTextContent("Fuel")
    expect(screen.getByText("2 of 2 proposed categories selected.")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Show the transactions of ALDI SAGT DANKE 2" }))
    const memberRow = screen.getByText("ALDI SAGT DANKE 1").closest("tr")!
    expect(within(memberRow).getByText("Fuel")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Accept 2 categories" }))
    await waitFor(() => expect(batchBodies(api)).toHaveLength(1))
    expect(batchBodies(api)[0]).toEqual(items.map((item) => ({ transactionId: item.id, categoryId: fuel, addMatchWords: [] })))
  })

  it("resets a group to its proposed category", async () => {
    const { api, user } = setup([aldi(2), aldi(1)])

    await user.click(await screen.findByRole("combobox", { name: "Category for all transactions of ALDI SAGT DANKE 2" }))
    await user.click(screen.getByRole("option", { name: "Fuel" }))
    await user.click(screen.getByRole("button", { name: "Back to the proposed category Groceries" }))

    expect(screen.getByRole("combobox", { name: "Category for all transactions of ALDI SAGT DANKE 2" })).toHaveTextContent("Groceries")
    await user.click(screen.getByRole("button", { name: "Accept 2 categories" }))
    await waitFor(() => expect(batchBodies(api)).toHaveLength(1))
    expect(batchBodies(api)[0].map((item) => item.categoryId)).toEqual([groceries, groceries])
  })

  it("groups keyword matches by counterparty and accepts them together with LLM suggestions", async () => {
    const shell = txn({ counterpartyName: "Shell", suggestion: { categoryId: fuel, confidence: "high" } })
    const items = [aldi(3), aldi(2), aldi(1), shell, txn({ counterpartyName: "Someone" })]
    const { api, user } = setup(items)

    const group = await screen.findByRole("checkbox", { name: "Select all transactions of ALDI SAGT DANKE 3" })
    expect(group).toBeChecked()
    expect(screen.getByText("3 transactions")).toBeInTheDocument()
    expect(screen.queryByText("ALDI SAGT DANKE 2")).toBeNull()
    expect(screen.getByText("4 of 4 proposed categories selected.")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Show the transactions of ALDI SAGT DANKE 3" }))
    expect(screen.getByText("ALDI SAGT DANKE 2")).toBeInTheDocument()

    const shellRow = screen.getByText("Shell").closest("tr")!
    await user.click(within(shellRow).getByRole("checkbox", { name: "Select category" }))
    await user.click(screen.getByRole("button", { name: "Accept 3 categories" }))

    await waitFor(() => expect(batchBodies(api)).toHaveLength(1))
    expect(batchBodies(api)[0]).toEqual(items.slice(0, 3).map((item) => ({ transactionId: item.id, categoryId: groceries, addMatchWords: [] })))
    expect(await screen.findByText("3 transactions reviewed.")).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByText("3 transactions")).toBeNull())
    expect(screen.getByText("Shell")).toBeInTheDocument()
  })

  it("deselects a whole group with its checkbox", async () => {
    const { user } = setup([aldi(2), aldi(1), txn({ counterpartyName: "Esso", categoryId: fuel, categorizationSource: "keyword" })])

    await user.click(await screen.findByRole("checkbox", { name: "Select all transactions of ALDI SAGT DANKE 2" }))

    expect(screen.getByText("1 of 3 proposed categories selected.")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Accept 1 category" })).toBeEnabled()
    await user.click(screen.getByRole("button", { name: "Select none" }))
    expect(screen.getByRole("button", { name: "Accept 0 categories" })).toBeDisabled()
  })

  it("splits a large selection into requests of at most 100", async () => {
    const items = Array.from({ length: 150 }, (_, i) => aldi((i % 28) + 1))
    const { api, user } = setup(items)

    await user.click(await screen.findByRole("button", { name: "Accept 150 categories" }))

    await waitFor(() => expect(batchBodies(api).map((body) => body.length)).toEqual([100, 50]))
  })

  it("pages through the queue", async () => {
    const items = Array.from({ length: 120 }, (_, i) => txn({ counterpartyName: `Payee ${i + 1}` }))
    const { user } = setup(items)

    expect(await screen.findByText("120 transactions to review")).toBeInTheDocument()
    expect(screen.getAllByText("Page 1 of 3")).toHaveLength(2)
    expect(screen.getByText("Payee 50")).toBeInTheDocument()
    expect(screen.queryByText("Payee 51")).toBeNull()

    await user.click(screen.getAllByRole("button", { name: "Next page" })[0])
    expect(screen.getAllByText("Page 2 of 3")).toHaveLength(2)
    expect(screen.getByText("Payee 51")).toBeInTheDocument()

    await user.click(screen.getAllByRole("button", { name: "Last page" })[0])
    expect(screen.getByText("Payee 120")).toBeInTheDocument()
    expect(screen.getAllByRole("button", { name: "Next page" })[0]).toBeDisabled()
  })

  it("clears the prefilled match word with one click", async () => {
    const { user } = setup([txn({ counterpartyName: "Aral 12", suggestion: { categoryId: fuel, confidence: "high", matchWord: "aral" } })])

    await user.click(await screen.findByText("Aral 12"))
    const input = screen.getByRole("textbox", { name: "Match words" })
    expect(input).toHaveValue("aral")

    await user.click(screen.getByRole("button", { name: "Clear match words" }))
    expect(input).toHaveValue("")
    expect(screen.queryByRole("button", { name: "Clear match words" })).toBeNull()
  })

  it("keeps the chunks stored before a failed request, shows one error and reloads the queue", async () => {
    const items = Array.from({ length: 150 }, (_, i) => aldi((i % 28) + 1))
    const { api, user } = setup(items, defaultCategories, 2)

    await user.click(await screen.findByRole("button", { name: "Accept 150 categories" }))

    await waitFor(() => expect(errorToasts()).toEqual(["database gone"]))
    expect(batchBodies(api).map((body) => body.length)).toEqual([100, 50])
    expect(await screen.findByText("50 transactions to review")).toBeInTheDocument()
    expect(api.callsTo("GET", "/ledger/review-queue?limit=all")).toHaveLength(2)
    expect(screen.getByRole("button", { name: "Accept 50 categories" })).toBeEnabled()
    expect(errorToasts()).toEqual(["database gone"])
  })

  it("stays on the last page that still exists when the queue shrinks", async () => {
    const letters = (i: number) => String.fromCharCode(97 + Math.floor(i / 26), 97 + (i % 26))
    // distinct counterparty words, so every transaction is a row of its own
    const proposed = Array.from({ length: 60 }, (_, i) => txn({ counterpartyName: `Shop${letters(i)}`, categoryId: groceries, categorizationSource: "keyword" }))
    const open = Array.from({ length: 60 }, (_, i) => txn({ counterpartyName: `Payee${letters(i)}` }))
    const { user } = setup([...proposed, ...open])

    expect(await screen.findByText("120 transactions to review")).toBeInTheDocument()
    await user.click(screen.getAllByRole("button", { name: "Last page" })[0])
    expect(screen.getAllByText("Page 3 of 3")).toHaveLength(2)

    await user.click(screen.getByRole("button", { name: "Accept 60 categories" }))

    expect(await screen.findByText("60 transactions to review")).toBeInTheDocument()
    expect(screen.getAllByText("Page 2 of 2")).toHaveLength(2)
    expect(screen.getByText("Payeeby")).toBeInTheDocument()
    expect(screen.getByText("Payeech")).toBeInTheDocument()
    expect(screen.queryByText("Payeebx")).toBeNull()
  })
})
