import { describe, expect, it, vi } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { mockApi } from "@/test/api-mock"
import type { CostEntry } from "@/modules/auto/types"
import type { PaperlessLink } from "@/types/paperless"
import { CostEntriesTable } from "@/modules/auto/components/cost-entries-table"

const at = "2026-11-01T10:00:00Z"

function entry(n: number): CostEntry {
  return {
    id: `cost-${n}`,
    vehicleId: "vehicle-1",
    type: "fuel",
    description: `Entry ${n}`,
    amount: 50,
    date: `2026-${String((n % 12) + 1).padStart(2, "0")}-01`,
    createdAt: at,
    updatedAt: at,
  }
}

function link(entityId: string, documentId: number, title: string): PaperlessLink {
  return { entityType: "cost", entityId, documentId, title, entityUrl: "https://kontor.example/x", createdAt: at }
}

const entries = Array.from({ length: 120 }, (_, i) => entry(i + 1))

function rowOf(description: string) {
  return screen.getByText(description).closest("tr")!
}

describe("CostEntriesTable paperless indicator", () => {
  it("shows the documents of entries that have some, from one request for the whole list", async () => {
    const api = mockApi({
      "GET /paperless/config": { configured: true },
      "GET /paperless/links/cost": [link("cost-1", 7, "Invoice fuel"), link("cost-1", 8, ""), link("cost-2", 9, "Receipt")],
    })
    const onDocuments = vi.fn()
    const { user } = renderWithProviders(
      <CostEntriesTable entries={entries} onEdit={vi.fn()} onDelete={vi.fn()} onDocuments={onDocuments} />,
    )

    const indicator = await within(rowOf("Entry 1")).findByRole("button", { name: "2 linked paperless documents" })
    expect(indicator).toHaveTextContent("2")
    expect(indicator).toHaveAttribute("title", "Invoice fuel\n#8")
    expect(within(rowOf("Entry 2")).getByRole("button", { name: "1 linked paperless document" })).toBeInTheDocument()
    expect(within(rowOf("Entry 3")).queryByRole("button", { name: /linked paperless/ })).toBeNull()

    await user.click(indicator)
    expect(onDocuments).toHaveBeenCalledWith(entries[0])

    expect(api.calls.filter((c) => c.path.startsWith("/paperless/links"))).toEqual([
      expect.objectContaining({ method: "GET", path: "/paperless/links/cost" }),
    ])
  })

  it("keeps the documents menu item", async () => {
    mockApi({ "GET /paperless/config": { configured: true }, "GET /paperless/links/cost": [] })
    const onDocuments = vi.fn()
    const { user } = renderWithProviders(
      <CostEntriesTable entries={[entry(1)]} onEdit={vi.fn()} onDelete={vi.fn()} onDocuments={onDocuments} />,
    )

    const buttons = within(rowOf("Entry 1")).getAllByRole("button")
    await user.click(buttons[buttons.length - 1])
    await user.click(await screen.findByRole("menuitem", { name: "Paperless documents" }))
    expect(onDocuments).toHaveBeenCalledWith(entry(1))
  })

  it("shows nothing and loads no links while paperless is not configured", async () => {
    const api = mockApi({ "GET /paperless/config": { configured: false }, "GET /paperless/links/cost": [link("cost-1", 7, "Invoice")] })
    renderWithProviders(<CostEntriesTable entries={entries} onEdit={vi.fn()} onDelete={vi.fn()} />)

    await waitFor(() => expect(api.callsTo("GET", "/paperless/config")).toHaveLength(1))
    expect(screen.queryByRole("button", { name: /linked paperless/ })).toBeNull()
    expect(api.callsTo("GET", "/paperless/links/cost")).toHaveLength(0)
  })
})
