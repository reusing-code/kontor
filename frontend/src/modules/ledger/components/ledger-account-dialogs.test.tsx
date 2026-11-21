import { describe, expect, it, vi } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { LedgerAccount } from "@/modules/ledger/types"
import { LedgerAccountDeleteDialog } from "@/modules/ledger/components/ledger-account-delete-dialog"
import { LedgerAccountEditDialog } from "@/modules/ledger/components/ledger-account-edit-dialog"

const at = "2026-11-01T10:00:00Z"
const account: LedgerAccount = { id: "acc-1", name: "Giro", bank: "DKB", iban: "DE89370400440532013000", currency: "EUR", createdAt: at, updatedAt: at }

describe("LedgerAccountDeleteDialog", () => {
  it("lists what goes before it asks", async () => {
    mockApi({
      "GET /ledger/accounts/acc-1/delete-summary": {
        transactions: 120,
        imports: 2,
        references: 1,
        transfers: 0,
        emailOrders: 3,
        bankAccounts: [{ connectionId: "c", bank: "DKB", uid: "u", name: "Girokonto", iban: "DE89…" }],
      },
    })
    const onConfirm = vi.fn()
    const { user } = renderWithProviders(<LedgerAccountDeleteDialog account={account} open onOpenChange={() => {}} onConfirm={onConfirm} />)

    expect(screen.getByRole("alertdialog", { name: "Delete Giro?" })).toBeInTheDocument()
    expect(await screen.findByText("120 transactions, with their categories, splits, notes and links")).toBeInTheDocument()
    expect(screen.getByText("2 imports; the files can be imported again")).toBeInTheDocument()
    expect(screen.getByText("1 link to a contract, purchase or vehicle (the item itself stays)")).toBeInTheDocument()
    expect(screen.getByText("the links of 3 email orders; they can be linked again")).toBeInTheDocument()
    expect(screen.getByText("the mapping of the bank account Girokonto at DKB")).toBeInTheDocument()
    expect(screen.queryByText(/internal transfer/)).toBeNull()

    await user.click(screen.getByRole("button", { name: "Delete account" }))
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it("does not offer deleting while the summary is missing", async () => {
    mockApi({})
    renderWithProviders(<LedgerAccountDeleteDialog account={account} open onOpenChange={() => {}} onConfirm={() => {}} />)
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument())
    expect(screen.getByRole("button", { name: "Delete account" })).toBeDisabled()
  })
})

describe("LedgerAccountEditDialog", () => {
  it("submits trimmed name and bank and refuses empty ones", async () => {
    const onSubmit = vi.fn()
    const { user } = renderWithProviders(<LedgerAccountEditDialog account={account} open onOpenChange={() => {}} onSubmit={onSubmit} />)

    const name = screen.getByRole("textbox", { name: "Account name" })
    await user.clear(name)
    await user.click(screen.getByRole("button", { name: "Save" }))
    expect(onSubmit).not.toHaveBeenCalled()

    await user.type(name, "  Tagesgeld ")
    await user.click(screen.getByRole("button", { name: "Save" }))
    await waitFor(() => expect(onSubmit).toHaveBeenCalledOnce())
    expect(onSubmit.mock.calls[0][0]).toEqual({ name: "Tagesgeld", bank: "DKB" })
  })
})
