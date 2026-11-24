import type { ReactNode } from "react"
import { describe, expect, it, vi } from "vitest"
import { screen, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { mockApi } from "@/test/api-mock"
import type { Contract } from "@/modules/contracts/types"
import { ContractsTable } from "@/modules/contracts/components/contracts-table"

vi.mock("@tanstack/react-router", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-router")>()),
  Link: ({ children, to, ...props }: { children: ReactNode; to: string }) => <a href={to} {...props}>{children}</a>,
}))

const at = "2026-11-01T10:00:00Z"

function contract(id: string, name: string): Contract {
  return {
    id, categoryId: "cat-1", name, billingInterval: "monthly", startDate: "2026-01-01",
    minimumDurationMonths: 12, extensionDurationMonths: 12, noticePeriodMonths: 1,
    paperlessUrl: "https://paperless.example/documents/1", createdAt: at, updatedAt: at,
  }
}

function rowOf(name: string) {
  return screen.getByText(name).closest("tr")!
}

describe("ContractsTable paperless indicator", () => {
  it("links to the contract page and replaces the legacy paperless link when documents are linked", async () => {
    mockApi({
      "GET /paperless/config": { configured: true },
      "GET /paperless/links/contract": [
        { entityType: "contract", entityId: "c-1", documentId: 3, title: "Contract PDF", entityUrl: "https://kontor.example/x", createdAt: at },
      ],
    })
    renderWithProviders(
      <ContractsTable contracts={[contract("c-1", "Internet"), contract("c-2", "Phone")]} onEdit={vi.fn()} onDelete={vi.fn()} />,
    )

    const indicator = await within(rowOf("Internet")).findByRole("link", { name: "1 linked paperless document" })
    expect(indicator).toHaveAttribute("href", "/contracts/c-1")
    expect(indicator).toHaveAttribute("title", "Contract PDF")
    expect(within(rowOf("Internet")).queryByTitle("Paperless URL")).toBeNull()

    expect(within(rowOf("Phone")).queryByRole("link", { name: /linked paperless/ })).toBeNull()
    expect(within(rowOf("Phone")).getByTitle("Paperless URL")).toBeInTheDocument()
  })
})
