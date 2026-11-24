import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { apiError, mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { LedgerEmailAccountsPanel } from "@/modules/ledger/components/ledger-email-accounts-panel"
import { draftKeys } from "@/lib/query-keys"

const account = {
  id: "7d0a1f6e-6a59-4b3a-9c0b-0d5c8f0f3a11",
  name: "Mail",
  imapHost: "imap.example.com",
  imapPort: 993,
  username: "me@example.com",
  useTls: true,
  folder: "Orders",
  preClassify: false,
  scanSince: "2026-01-01",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
}

function renderPanel(draftsFolder: string | undefined, run: unknown) {
  const api = mockApi({
    "GET /capabilities": { llm: true, llmVision: false, bankImport: false },
    "GET /ledger/email-accounts": [{ ...account, draftsFolder }],
    [`GET /ledger/email-accounts/${account.id}/scan`]: { state: "idle", emailsScanned: 0, ordersNew: 0, ordersLinked: 0, skipped: 0, warnings: [] },
    "POST /ledger/email-drafts/run": run,
  })
  return { ...renderWithProviders(<LedgerEmailAccountsPanel />), api }
}

describe("LedgerEmailAccountsPanel Kontor inbox folders", () => {
  it("offers no check without a folder for the Kontor inbox", async () => {
    renderPanel(undefined, {})
    expect(await screen.findByText("Mail")).toBeInTheDocument()
    expect(screen.queryByRole("button", { name: "Check Kontor inbox folders" })).not.toBeInTheDocument()
  })

  it("runs the check, reports the result and refreshes the drafts", async () => {
    const { user, api, queryClient } = renderPanel("Kontor", {
      found: 3, created: 2, skipped: 0, failed: 1, warnings: ["Mail: could not read the drafts folder"],
    })
    queryClient.setQueryData(draftKeys.summary(), { pending: 0, total: 0 })
    expect(await screen.findByText("Kontor inbox folder: Kontor")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Check Kontor inbox folders" }))
    expect(await screen.findByText("2 new drafts in the inbox.")).toBeInTheDocument()
    expect(screen.getByText("1 email could not be processed and is retried later.")).toBeInTheDocument()
    expect(screen.getByText("Mail: could not read the drafts folder")).toBeInTheDocument()
    expect(api.callsTo("POST", "/ledger/email-drafts/run")).toHaveLength(1)
    await waitFor(() => expect(queryClient.getQueryState(["drafts", "summary"])?.isInvalidated).toBe(true))
  })

  it("shows the server's reason when the check cannot run", async () => {
    const { user } = renderPanel("Kontor", apiError(409, "the LLM is not configured"))
    await user.click(await screen.findByRole("button", { name: "Check Kontor inbox folders" }))
    expect(await screen.findByText("the LLM is not configured")).toBeInTheDocument()
  })
})
