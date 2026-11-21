import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerCategory, LedgerTransaction } from "@/modules/ledger/types"
import { LedgerTransactionDetailsCard } from "@/modules/ledger/components/ledger-transaction-details-card"

const at = "2026-11-01T10:00:00Z"
const category = (id: string, name: string, parentId?: string): LedgerCategory => ({ id, name, parentId, matchWords: [], createdAt: at, updatedAt: at })

const txn: LedgerTransaction = {
  id: "txn-1",
  accountId: "acc",
  bookingDate: "2026-11-02",
  amountMinor: -17235,
  currency: "EUR",
  counterpartyName: "Marktkauf",
  categoryId: "groceries",
  reviewStatus: "confirmed",
  categorizationSource: "manual",
  sourceType: "dkb.csv",
  importBatchId: "batch",
  fingerprint: "fp",
  createdAt: at,
  updatedAt: at,
}

function setup(transaction: LedgerTransaction) {
  const api = mockApi({
    "GET /settings": { renewalDays: 30, reminderFrequency: "weekly", enabledModules: ["ledger"] },
    "GET /ledger/categories": [category("food", "Food"), category("groceries", "Groceries", "food"), category("cash", "Cash")],
    "GET /ledger/email-orders": [],
    "GET /ledger/transactions/txn-1/transfer-candidates": { items: [] },
    "GET /paperless/config": { configured: false },
    "POST /ledger/transactions/txn-1/review": { transaction: { ...transaction, categoryId: "cash" } },
  })
  return { api, ...renderWithProviders(<LedgerTransactionDetailsCard transaction={transaction} />) }
}

describe("LedgerTransactionDetailsCard category", () => {
  it("changes the category through a review", async () => {
    const { api, user } = setup(txn)

    const select = await screen.findByRole("combobox", { name: "Category" })
    await waitFor(() => expect(select).toHaveTextContent("Food › Groceries"))
    await user.click(select)
    await user.click(screen.getByRole("option", { name: "Cash" }))

    await waitFor(() => expect(api.callsTo("POST", "/ledger/transactions/txn-1/review")).toHaveLength(1))
    expect(api.callsTo("POST", "/ledger/transactions/txn-1/review")[0].body).toEqual({ categoryId: "cash", addMatchWords: [] })
    expect(await screen.findByText("Category changed.")).toBeInTheDocument()
  })

  it("keeps the category of a linked transfer read-only and offers no split", async () => {
    setup({ ...txn, categoryId: undefined, transferPairTransactionId: "00000000-0000-4000-8000-000000000001", specialCategory: "internalTransfer" })

    expect(await screen.findByText("A linked internal transfer cannot be split. Unlink it first.")).toBeInTheDocument()
    expect(screen.queryByRole("combobox", { name: "Category" })).toBeNull()
  })
})
