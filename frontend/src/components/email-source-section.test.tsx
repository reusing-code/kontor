import { describe, expect, it } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import { mockApi } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"
import { EmailSourceSection } from "@/components/email-source-section"

const entityId = "5b1c9d1e-4c0e-4a57-8a7e-0d1a2b3c4d5e"
const linksRoute = `GET /ledger/email-links/contract/${entityId}`
const link = {
  entityType: "contract",
  entityId,
  emailAccountId: "7d0a1f6e-6a59-4b3a-9c0b-0d5c8f0f3a11",
  accountName: "Mail",
  messageId: "<contract 1@provider.example>",
  subject: "Your contract",
  from: "Provider <service@provider.example>",
  date: "2026-11-01T10:00:00Z",
  createdAt: "2026-11-02T10:00:00Z",
}

function renderSection(links: unknown[], enabledModules: string[], props: { showEmpty?: boolean } = {}) {
  const api = mockApi({ "GET /settings": { enabledModules }, [linksRoute]: links })
  return { ...renderWithProviders(<EmailSourceSection entityType="contract" entityId={entityId} {...props} />), api }
}

describe("EmailSourceSection", () => {
  it("shows the email with a mid: link built from the Message-ID", async () => {
    renderSection([link], ["ledger", "contracts"])
    const anchor = await screen.findByRole("link", { name: "Your contract" })
    expect(anchor).toHaveAttribute("href", "mid:contract%201%40provider.example")
    expect(screen.getByText(/Provider <service@provider.example> · .* · Mail/)).toBeInTheDocument()
    expect(screen.getByText("Source email")).toBeInTheDocument()
  })

  it("shows plain text without a usable Message-ID and a placeholder without a subject", async () => {
    renderSection([{ ...link, messageId: undefined, subject: undefined }], ["ledger"])
    expect(await screen.findByText("(no subject)")).toBeInTheDocument()
    expect(screen.queryByRole("link")).not.toBeInTheDocument()
  })

  it("renders nothing without a source email unless asked to", async () => {
    const { api, container, unmount } = renderSection([], ["ledger"])
    await waitFor(() => expect(api.callsTo("GET", `/ledger/email-links/contract/${entityId}`)).toHaveLength(1))
    expect(container.querySelector("[data-slot=card]")).toBeNull()
    expect(screen.queryByText("Source email")).not.toBeInTheDocument()
    unmount()

    renderSection([], ["ledger"], { showEmpty: true })
    expect(await screen.findByText("Not created from an email.")).toBeInTheDocument()
  })

  it("does not ask for the links while the ledger module is disabled", async () => {
    const { api } = renderSection([link], ["contracts"])
    await waitFor(() => expect(api.callsTo("GET", "/settings")).toHaveLength(1))
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(api.callsTo("GET", `/ledger/email-links/contract/${entityId}`)).toHaveLength(0)
    expect(screen.queryByText("Your contract")).not.toBeInTheDocument()
  })
})
