import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { LedgerEmailOrdersTable } from "@/modules/ledger/components/ledger-email-orders-table"
import type { LedgerEmailOrder } from "@/modules/ledger/types"

const order: LedgerEmailOrder = {
  id: "0a1b2c3d-0000-4000-8000-000000000001",
  emailAccountId: "7d0a1f6e-6a59-4b3a-9c0b-0d5c8f0f3a11",
  importerId: "amazon",
  externalOrderId: "302-1",
  orderDate: "2026-03-01",
  totalMinor: 1999,
  currency: "EUR",
  merchant: "Amazon",
  matchStatus: "unmatched",
  createdAt: "2026-03-01T00:00:00Z",
  updatedAt: "2026-03-01T00:00:00Z",
}
const rejectPath = `/ledger/email-orders/${order.id}/reject`

function renderTable() {
  const api = mockApi({
    "GET /ledger/email-accounts": [],
    [`POST ${rejectPath}`]: { ...order, matchStatus: "rejected" },
  })
  return { ...renderWithProviders(<LedgerEmailOrdersTable orders={[order]} />), api }
}

describe("LedgerEmailOrdersTable reject", () => {
  it("rejects an order after asking", async () => {
    const { user, api } = renderTable()

    await user.click(screen.getByRole("button", { name: "Reject" }))
    const confirm = await screen.findByRole("alertdialog")
    expect(confirm).toHaveTextContent('Reject the email order "Amazon · 302-1"?')
    expect(api.callsTo("POST", rejectPath)).toHaveLength(0)

    await user.click(within(confirm).getByRole("button", { name: "Reject" }))
    expect(await screen.findByText("Email order rejected.")).toBeInTheDocument()
    expect(api.callsTo("POST", rejectPath)).toHaveLength(1)
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
  })

  it("sends nothing when rejecting is cancelled", async () => {
    const { user, api } = renderTable()

    await user.click(screen.getByRole("button", { name: "Reject" }))
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(api.callsTo("POST", rejectPath)).toHaveLength(0)
  })
})
