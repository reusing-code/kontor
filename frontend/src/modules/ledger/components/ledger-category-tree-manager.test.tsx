import { describe, expect, it } from "vitest"
import { screen, within } from "@testing-library/react"
import { renderWithProviders } from "@/test/render"
import { mockApi } from "@/test/api-mock"
import type { LedgerCategory } from "@/modules/ledger/types"
import { LedgerCategoryTreeManager } from "@/modules/ledger/components/ledger-category-tree-manager"

function category(id: string, name: string, parentId?: string, matchWords: string[] = []): LedgerCategory {
  const at = "2026-11-01T10:00:00Z"
  return { id, name, parentId, matchWords, createdAt: at, updatedAt: at }
}

const categories = [
  category("food", "Food"),
  category("groceries", "Groceries", "food", ["rewe", "edeka"]),
  category("restaurants", "Restaurants", "food"),
  category("fastfood", "Fast food", "restaurants"),
  category("rent", "Rent"),
]

describe("LedgerCategoryTreeManager", () => {
  it("starts fully expanded", () => {
    mockApi({})
    renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    for (const name of ["Food", "Groceries", "Restaurants", "Fast food", "Rent"]) {
      expect(screen.getByText(name)).toBeInTheDocument()
    }
    expect(screen.getByText("rewe")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Collapse Food" })).toHaveAttribute("aria-expanded", "true")
    expect(screen.queryByRole("button", { name: /Rent$/, expanded: true })).toBeNull()
  })

  it("collapses and expands a parent with its whole subtree", async () => {
    mockApi({})
    const { user } = renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    await user.click(screen.getByRole("button", { name: "Collapse Food" }))

    expect(screen.queryByText("Groceries")).toBeNull()
    expect(screen.queryByText("Fast food")).toBeNull()
    expect(screen.getByText("2 subcategories")).toBeInTheDocument()
    expect(screen.getByText("Rent")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Expand Food" }))

    expect(screen.getByText("Groceries")).toBeInTheDocument()
    expect(screen.getByText("Fast food")).toBeInTheDocument()
  })

  it("keeps a collapsed child collapsed when its parent is reopened", async () => {
    mockApi({})
    const { user } = renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    await user.click(screen.getByRole("button", { name: "Collapse Restaurants" }))
    await user.click(screen.getByRole("button", { name: "Collapse Food" }))
    await user.click(screen.getByRole("button", { name: "Expand Food" }))

    expect(screen.getByText("Restaurants")).toBeInTheDocument()
    expect(screen.queryByText("Fast food")).toBeNull()
    expect(screen.getByText("1 subcategory")).toBeInTheDocument()
  })

  it("collapses and expands everything from the header", async () => {
    mockApi({})
    const { user } = renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    await user.click(screen.getByRole("button", { name: "Collapse all" }))
    expect(screen.queryByText("Groceries")).toBeNull()
    expect(screen.getByText("Food")).toBeInTheDocument()
    expect(screen.getByText("Rent")).toBeInTheDocument()

    await user.click(screen.getByRole("button", { name: "Expand all" }))
    expect(screen.getByText("Fast food")).toBeInTheDocument()
  })

  it("hides the expand and collapse buttons for a flat list", () => {
    mockApi({})
    renderWithProviders(<LedgerCategoryTreeManager categories={[category("rent", "Rent")]} />)

    expect(screen.queryByRole("button", { name: "Collapse all" })).toBeNull()
    expect(screen.getByRole("button", { name: "Edit Rent" })).toBeInTheDocument()
  })

  it("opens the create dialog with the row as parent", async () => {
    mockApi({})
    const { user } = renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    await user.click(screen.getByRole("button", { name: "Add subcategory to Rent" }))

    expect(await screen.findByRole("dialog")).toHaveTextContent("Create ledger category")
  })

  it("offers the parent as a tree without the category's own subtree", async () => {
    mockApi({})
    const { user } = renderWithProviders(<LedgerCategoryTreeManager categories={categories} />)

    await user.click(screen.getByRole("button", { name: "Edit Restaurants" }))
    const dialog = await screen.findByRole("dialog")
    const parent = within(dialog).getByRole("combobox", { name: "Parent category" })
    expect(parent).toHaveTextContent("Food")

    await user.click(parent)
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["No parent", "Food", "Groceries", "Rent"])
  })
})
