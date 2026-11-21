import { describe, expect, it } from "vitest"
import type { LedgerCategory } from "@/modules/ledger/types"
import { descendantIds, flattenCategoryTree } from "@/modules/ledger/lib/category-tree"

function category(id: string, name: string, parentId?: string): LedgerCategory {
  const at = "2026-11-01T10:00:00Z"
  return { id, name, parentId, matchWords: [], createdAt: at, updatedAt: at }
}

describe("flattenCategoryTree", () => {
  it("lists depth-first with siblings by name and the path to each category", () => {
    const tree = flattenCategoryTree([
      category("rent", "Rent"),
      category("rest", "Restaurants", "food"),
      category("food", "Food"),
      category("fast", "Fast food", "rest"),
      category("gro", "Groceries", "food"),
    ])
    expect(tree.map((e) => [e.category.id, e.depth, e.path.join("/")])).toEqual([
      ["food", 0, "Food"],
      ["gro", 1, "Food/Groceries"],
      ["rest", 1, "Food/Restaurants"],
      ["fast", 2, "Food/Restaurants/Fast food"],
      ["rent", 0, "Rent"],
    ])
  })

  it("keeps categories with a missing parent or in a cycle", () => {
    const tree = flattenCategoryTree([category("orphan", "Orphan", "gone"), category("a", "A", "b"), category("b", "B", "a")])
    expect(tree.map((e) => e.category.id).sort()).toEqual(["a", "b", "orphan"])
  })
})

describe("descendantIds", () => {
  it("returns the category and its whole subtree", () => {
    const categories = [category("food", "Food"), category("rest", "Restaurants", "food"), category("fast", "Fast food", "rest"), category("rent", "Rent")]
    expect([...descendantIds(categories, "food")].sort()).toEqual(["fast", "food", "rest"])
  })
})
