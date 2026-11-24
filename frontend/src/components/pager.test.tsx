import { describe, expect, it, vi } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { Pager } from "@/components/pager"

function pageButtons(): string[] {
  return screen.getAllByRole("button", { name: /^Page \d+$/ }).map((button) => button.textContent ?? "")
}

describe("Pager", () => {
  it("renders nothing for a single page", () => {
    renderWithProviders(<Pager page={1} pageCount={1} onChange={() => {}} label="Pages" />)
    expect(screen.queryByRole("navigation")).toBeNull()
  })

  it("shows the pages around the current one and marks it", () => {
    renderWithProviders(<Pager page={10} pageCount={20} onChange={() => {}} label="Pages" />)
    expect(screen.getByRole("navigation", { name: "Pages" })).toBeInTheDocument()
    expect(pageButtons()).toEqual(["1", "9", "10", "11", "20"])
    expect(screen.getByRole("button", { name: "Page 10" })).toHaveAttribute("aria-current", "page")
    expect(screen.getByRole("button", { name: "Page 9" })).not.toHaveAttribute("aria-current")
  })

  it("jumps to a page and steps back and forth", async () => {
    const onChange = vi.fn()
    const { user } = renderWithProviders(<Pager page={2} pageCount={5} onChange={onChange} label="Pages" />)

    await user.click(screen.getByRole("button", { name: "Page 5" }))
    await user.click(screen.getByRole("button", { name: "Previous page" }))
    await user.click(screen.getByRole("button", { name: "Next page" }))
    await user.click(screen.getByRole("button", { name: "Page 2" }))
    expect(onChange.mock.calls).toEqual([[5], [1], [3]])
  })

  it("disables stepping past the ends and everything while disabled", () => {
    const { unmount } = renderWithProviders(<Pager page={1} pageCount={3} onChange={() => {}} label="Pages" />)
    expect(screen.getByRole("button", { name: "Previous page" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Next page" })).toBeEnabled()
    unmount()

    renderWithProviders(<Pager page={3} pageCount={3} onChange={() => {}} label="Pages" disabled />)
    expect(screen.getByRole("button", { name: "Next page" })).toBeDisabled()
    expect(screen.getByRole("button", { name: "Page 1" })).toBeDisabled()
  })
})
