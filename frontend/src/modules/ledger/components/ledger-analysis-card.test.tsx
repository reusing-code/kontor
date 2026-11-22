import { describe, expect, it } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"

const props = { title: "Card", isPending: false, isError: false, onRetry: () => {}, isEmpty: false, emptyText: "Nothing here." }

describe("LedgerAnalysisCard", () => {
  it("marks content of the previous range as busy", () => {
    const { rerender } = renderWithProviders(
      <LedgerAnalysisCard {...props} isRefreshing footer={<p>Footer</p>}>
        <p>Content</p>
      </LedgerAnalysisCard>,
    )
    const content = screen.getByText("Content").parentElement
    expect(content).toHaveAttribute("aria-busy", "true")
    expect(content).toHaveClass("opacity-60")
    expect(content).toContainElement(screen.getByText("Footer"))

    rerender(
      <LedgerAnalysisCard {...props}>
        <p>Content</p>
      </LedgerAnalysisCard>,
    )
    expect(screen.getByText("Content").parentElement).toHaveAttribute("aria-busy", "false")
    expect(screen.getByText("Content").parentElement).not.toHaveClass("opacity-60")
  })

  it("shows the empty text with the footer, and the error instead of both", () => {
    const { rerender } = renderWithProviders(
      <LedgerAnalysisCard {...props} isEmpty footer={<p>Footer</p>}>
        <p>Content</p>
      </LedgerAnalysisCard>,
    )
    expect(screen.getByText("Nothing here.")).toBeInTheDocument()
    expect(screen.getByText("Footer")).toBeInTheDocument()
    expect(screen.queryByText("Content")).toBeNull()

    rerender(
      <LedgerAnalysisCard {...props} isError footer={<p>Footer</p>}>
        <p>Content</p>
      </LedgerAnalysisCard>,
    )
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument()
    expect(screen.queryByText("Footer")).toBeNull()
  })
})
