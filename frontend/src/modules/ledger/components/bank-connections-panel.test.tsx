import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import type { BankAccount, BankConnection, LedgerAccount } from "@/modules/ledger/types"
import { BankConnectionsPanel } from "@/modules/ledger/components/bank-connections-panel"

const at = "2026-11-01T10:00:00Z"
const ledger = (id: string, name: string): LedgerAccount => ({ id, name, bank: "DKB", currency: "EUR", createdAt: at, updatedAt: at })

function bankAccount(fields: Partial<BankAccount>): BankAccount {
  return { uid: "uid-1", iban: "DE89370400440532013000", currency: "EUR", name: "Girokonto", mapped: false, lastSyncStatus: "", statusMessage: "", lastImported: 0, lastDuplicates: 0, ...fields }
}

function setup(accounts: BankAccount[], balancesRequested = true) {
  let connection: BankConnection = { id: "conn-1", aspspName: "DKB", aspspCountry: "DE", authorizedAt: at, expiresAt: "2027-02-01T00:00:00Z", status: "active", balancesRequested, accounts }
  const api = mockApi({
    "GET /capabilities": { llm: false, llmVision: false, bankImport: true },
    "GET /ledger/accounts": [ledger("giro", "Giro"), ledger("savings", "Savings")],
    "GET /ledger/bank/connections": () => [connection],
    "DELETE /ledger/bank/connections/conn-1/accounts/uid-1": () => {
      connection = { ...connection, accounts: [bankAccount({})] }
      return connection
    },
    "PUT /ledger/bank/connections/conn-1/accounts/uid-1": (call: ApiCall) => {
      const { ledgerAccountId } = call.body as { ledgerAccountId: string }
      connection = { ...connection, accounts: [bankAccount({ mapped: true, ledgerAccountId })] }
      return connection
    },
  })
  return { api, ...renderWithProviders(<BankConnectionsPanel />) }
}

describe("BankConnectionsPanel mapping", () => {
  it("unmaps a mapped bank account", async () => {
    const { api, user } = setup([bankAccount({ mapped: true, ledgerAccountId: "giro" })])

    expect(await screen.findByText("Mapped to Giro")).toBeInTheDocument()
    await user.click(screen.getByRole("button", { name: "Unmap" }))

    await waitFor(() => expect(api.callsTo("DELETE", "/ledger/bank/connections/conn-1/accounts/uid-1")).toHaveLength(1))
    expect(await screen.findByText("Mapping removed. The bank account is not synced until it is mapped again.")).toBeInTheDocument()
    expect(await screen.findByRole("combobox", { name: "Map to ledger account" })).toBeInTheDocument()
  })

  it("changes the mapping to another ledger account", async () => {
    const { api, user } = setup([bankAccount({ mapped: true, ledgerAccountId: "giro" })])

    await user.click(await screen.findByRole("button", { name: "Change" }))
    expect(screen.getByText(/Transactions already synced stay in their ledger account/)).toBeInTheDocument()
    await user.click(screen.getByRole("combobox", { name: "Map to ledger account" }))
    await user.click(screen.getByRole("option", { name: "Savings" }))

    await waitFor(() => expect(api.callsTo("PUT", "/ledger/bank/connections/conn-1/accounts/uid-1")).toHaveLength(1))
    expect(api.callsTo("PUT", "/ledger/bank/connections/conn-1/accounts/uid-1")[0].body).toEqual({ ledgerAccountId: "savings" })
    expect(await screen.findByText("Mapped to Savings")).toBeInTheDocument()
    expect(screen.queryByRole("combobox", { name: "Map to ledger account" })).toBeNull()
  })
})

describe("BankConnectionsPanel balances", () => {
  it("asks to re-authorize a consent from before balances were requested", async () => {
    setup([bankAccount({ mapped: true, ledgerAccountId: "giro", lastSyncStatus: "ok", lastBalanceStatus: "unavailable" })], false)
    expect(await screen.findByText(/This consent was given before Kontor read balances/)).toBeInTheDocument()
  })

  it("says when the bank reports no booked balance", async () => {
    setup([bankAccount({ mapped: true, ledgerAccountId: "giro", lastSyncStatus: "ok", lastBalanceStatus: "unavailable" })])
    expect(await screen.findByText("The bank reports no booked balance for this account.")).toBeInTheDocument()
  })

  it("shows nothing when the balance was loaded", async () => {
    setup([bankAccount({ mapped: true, ledgerAccountId: "giro", lastSyncStatus: "ok", lastBalanceStatus: "ok" })], false)
    expect(await screen.findByText("Mapped to Giro")).toBeInTheDocument()
    expect(screen.queryByText(/Re-authorize to load the balance/)).toBeNull()
  })
})
