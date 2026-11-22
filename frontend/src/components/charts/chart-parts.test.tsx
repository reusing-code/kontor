import { describe, expect, it, vi } from "vitest"
import { render, screen } from "@testing-library/react"
import { ChartLegend, ChartTooltipBox } from "@/components/charts/chart-parts"

describe("chart parts", () => {
  // Names come from user data: two accounts or categories can share one.
  it("renders entries with the same name without a React key warning", () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {})

    render(
      <>
        <ChartLegend
          items={[
            { label: "Girokonto", color: "red" },
            { label: "Girokonto", color: "blue" },
          ]}
        />
        <ChartTooltipBox
          title="Nov 26"
          rows={[
            { name: "Total", value: "€1.00", color: "red" },
            { name: "Total", value: "€2.00" },
          ]}
        />
      </>,
    )

    expect(screen.getAllByText("Girokonto")).toHaveLength(2)
    expect(screen.getAllByText("Total")).toHaveLength(2)
    expect(screen.getByText("€1.00")).toBeInTheDocument()
    expect(screen.getByText("€2.00")).toBeInTheDocument()
    expect(errors).not.toHaveBeenCalled()
  })
})
