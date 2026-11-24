import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { apiError, mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { errorToasts } from "@/test/toasts"
import type { LedgerEmailOrder, LedgerTransaction } from "@/modules/ledger/types"
import { PurchaseFromTransaction } from "@/modules/ledger/components/ledger-transaction-create-actions"

const at = "2026-11-01T10:00:00Z"
const categoryId = "11111111-1111-4111-8111-111111111111"

const txn: LedgerTransaction = {
  id: "txn-1",
  accountId: "acc",
  bookingDate: "2026-11-02",
  amountMinor: -5998,
  currency: "EUR",
  counterpartyName: "Shop",
  reviewStatus: "confirmed",
  categorizationSource: "manual",
  sourceType: "dkb.csv",
  importBatchId: "batch",
  fingerprint: "fp",
  createdAt: at,
  updatedAt: at,
}

const order: LedgerEmailOrder = {
  id: "22222222-2222-4222-8222-222222222222",
  emailAccountId: "33333333-3333-4333-8333-333333333333",
  importerId: "shop",
  orderDate: "2026-10-30",
  totalMinor: 5998,
  currency: "EUR",
  merchant: "Shop",
  items: [
    { name: "Desk lamp", quantity: 1, unitPriceMinor: 2999, totalPriceMinor: 2999 },
    { name: "Bookshelf", quantity: 1, unitPriceMinor: 2999, totalPriceMinor: 2999 },
  ],
  matchStatus: "matched",
  createdAt: at,
  updatedAt: at,
}

describe("PurchaseFromTransaction", () => {
  it("keeps a failed purchase open with its input and moves on after the retry", async () => {
    let attempts = 0
    const api = mockApi({
      "GET /modules/purchases/categories": [{ id: categoryId, name: "Home", createdAt: at, updatedAt: at }],
      [`POST /categories/${categoryId}/purchases`]: () =>
        ++attempts === 1 ? apiError(400, "name taken") : { id: `purchase-${attempts}` },
    })
    const link = vi.fn(async () => true)
    const { user } = renderWithProviders(<PurchaseFromTransaction transaction={txn} orders={[order]} link={link} />)

    await user.click(screen.getByRole("combobox"))
    await user.click(await screen.findByRole("option", { name: "Home" }))
    await user.click(screen.getByRole("button", { name: "Create purchase from this transaction" }))
    await user.click(await screen.findByRole("button", { name: "Continue with 2 items" }))

    const dialog = await screen.findByRole("dialog", { name: "Add Purchase" })
    const name = within(dialog).getByDisplayValue("Desk lamp")
    await user.type(name, " typed")
    await user.click(within(dialog).getByRole("button", { name: "Save" }))

    await waitFor(() => expect(errorToasts()).toEqual(["name taken"]))
    expect(screen.getByRole("dialog", { name: "Add Purchase" })).toBe(dialog)
    expect(within(dialog).getByDisplayValue("Desk lamp typed")).toBeInTheDocument()
    expect(link).not.toHaveBeenCalled()

    const save = within(dialog).getByRole("button", { name: "Save" })
    await waitFor(() => expect(save).toBeEnabled())
    await user.click(save)

    // the next item's form follows; closing it skips that item
    expect(await screen.findByDisplayValue("Bookshelf")).toBeInTheDocument()
    expect(link).toHaveBeenCalledExactlyOnceWith({ type: "purchase", targetId: "purchase-2" })
    await user.click(within(screen.getByRole("dialog", { name: "Add Purchase" })).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument())
    expect(await screen.findByText("1 purchase created and linked.")).toBeInTheDocument()
    const creates = api.callsTo("POST", `/categories/${categoryId}/purchases`)
    expect(creates).toHaveLength(2)
    expect(creates[1].body).toMatchObject({ itemName: "Desk lamp typed" })
    expect(errorToasts()).toEqual(["name taken"])
  })
})
