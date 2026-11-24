import { describe, expect, it } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { mockApi } from "@/test/api-mock"
import { PaperlessDocumentsSection } from "@/components/paperless/paperless-documents-section"

const contractId = "c0ffee00-0000-4000-8000-000000000001"
const detachPath = `/paperless/links/contract/${contractId}/42`

function renderSection() {
  const api = mockApi({
    "GET /paperless/config": { configured: true, baseUrl: "https://paperless.example" },
    [`GET /paperless/links/contract/${contractId}`]: [
      { entityType: "contract", entityId: contractId, documentId: 42, title: "Contract scan", entityUrl: "/contracts/x", createdAt: "2026-01-01T00:00:00Z" },
    ],
    [`DELETE ${detachPath}`]: undefined,
  })
  return { ...renderWithProviders(<PaperlessDocumentsSection entityType="contract" entityId={contractId} entityUrl="/contracts/x" />), api }
}

describe("PaperlessDocumentsSection detach", () => {
  it("removes the document link after asking", async () => {
    const { user, api } = renderSection()

    await user.click(await screen.findByRole("button", { name: 'Remove the link to "Contract scan"' }))
    const confirm = await screen.findByRole("alertdialog")
    expect(confirm).toHaveTextContent('Remove the link to the document "Contract scan"?')
    expect(api.callsTo("DELETE", detachPath)).toHaveLength(0)

    await user.click(within(confirm).getByRole("button", { name: "Remove document link" }))
    expect(await screen.findByText("Document link removed")).toBeInTheDocument()
    expect(api.callsTo("DELETE", detachPath)).toHaveLength(1)
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
  })

  it("sends nothing when detaching is cancelled", async () => {
    const { user, api } = renderSection()

    await user.click(await screen.findByRole("button", { name: 'Remove the link to "Contract scan"' }))
    await user.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Cancel" }))

    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument())
    expect(api.callsTo("DELETE", detachPath)).toHaveLength(0)
  })
})
