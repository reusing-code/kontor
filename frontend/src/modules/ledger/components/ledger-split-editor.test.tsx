import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerCategory, LedgerTransaction, LedgerTransactionSplit } from "@/modules/ledger/types"
import { LedgerSplitEditor } from "@/modules/ledger/components/ledger-split-editor"

const at = "2026-11-01T10:00:00Z"
const category = (id: string, name: string, parentId?: string): LedgerCategory => ({ id, name, parentId, matchWords: [], createdAt: at, updatedAt: at })
const categories = [category("groceries", "Groceries"), category("cash", "Cash")]

function transaction(fields: Partial<LedgerTransaction> = {}): LedgerTransaction {
  return {
    id: "txn-1",
    accountId: "acc",
    bookingDate: "2026-11-02",
    amountMinor: -17235,
    currency: "EUR",
    categoryId: "groceries",
    reviewStatus: "confirmed",
    categorizationSource: "manual",
    sourceType: "dkb.csv",
    importBatchId: "batch",
    fingerprint: "fp",
    createdAt: at,
    updatedAt: at,
    ...fields,
  }
}

function setup(txn: LedgerTransaction) {
  const api = mockApi({
    "PUT /ledger/transactions/txn-1/splits": (call: ApiCall) => ({ ...txn, splits: (call.body as { splits: LedgerTransactionSplit[] }).splits }),
  })
  return { api, ...renderWithProviders(<LedgerSplitEditor transaction={txn} categories={categories} />) }
}

const sent = (api: ReturnType<typeof mockApi>) => api.callsTo("PUT", "/ledger/transactions/txn-1/splits").map((call) => call.body)

describe("LedgerSplitEditor", () => {
  it("splits off a part and shows what stays with the main category", async () => {
    const { api, user } = setup(transaction())

    await user.click(screen.getByRole("button", { name: "Add part" }))
    expect(screen.getByRole("button", { name: "Save split" })).toBeDisabled()

    await user.type(screen.getByRole("textbox", { name: "Amount of part 1" }), "150,00")
    await user.click(screen.getByRole("combobox", { name: "Category of part 1" }))
    await user.click(screen.getByRole("option", { name: "Cash" }))
    await user.type(screen.getByRole("textbox", { name: "Note of part 1" }), "Auszahlung")

    expect(screen.getByText("Remaining -€22.35 stays with Groceries.")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Save split" }))

    await waitFor(() => expect(sent(api)).toEqual([{ splits: [{ amountMinor: -15000, categoryId: "cash", note: "Auszahlung" }] }]))
    expect(await screen.findByText("Split saved.")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Save split" })).toBeNull()
  })

  it("refuses parts that use up the whole amount", async () => {
    const { user } = setup(transaction())

    await user.click(screen.getByRole("button", { name: "Add part" }))
    await user.type(screen.getByRole("textbox", { name: "Amount of part 1" }), "172,35")
    await user.click(screen.getByRole("combobox", { name: "Category of part 1" }))
    await user.click(screen.getByRole("option", { name: "Cash" }))

    expect(screen.getByText("The parts must add up to less than the transaction amount.")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Save split" })).toBeDisabled()
  })

  it("removes an existing split", async () => {
    const { api, user } = setup(transaction({ splits: [{ amountMinor: -15000, categoryId: "cash" }] }))

    expect(screen.getByRole("textbox", { name: "Amount of part 1" })).toHaveValue("150.00")
    await user.click(screen.getByRole("button", { name: "Remove part 1" }))
    await user.click(screen.getByRole("button", { name: "Remove split" }))

    await waitFor(() => expect(sent(api)).toEqual([{ splits: [] }]))
    expect(await screen.findByText("Split removed.")).toBeInTheDocument()
  })

  it("asks for a category again for a part whose category was deleted", () => {
    setup(transaction({ splits: [{ amountMinor: -15000 }] }))
    expect(screen.getByText("Choose a category for every part.")).toBeInTheDocument()
  })

  it("does not offer splitting a linked transfer", () => {
    setup(transaction({ transferPairTransactionId: "00000000-0000-4000-8000-000000000001" }))
    expect(screen.getByText("A linked internal transfer cannot be split. Unlink it first.")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Add part" })).toBeNull()
  })
})
