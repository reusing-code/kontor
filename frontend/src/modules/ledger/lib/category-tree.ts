import type { LedgerCategory } from "@/modules/ledger/types"

export interface CategoryTreeEntry {
  category: LedgerCategory
  depth: number
  // names from the root down to the category itself
  path: string[]
}

// flattenCategoryTree lists the categories depth-first, siblings by name.
// A category whose parent is missing counts as a root; categories caught in a
// parent cycle are appended as roots, so every category shows up once.
export function flattenCategoryTree(categories: LedgerCategory[]): CategoryTreeEntry[] {
  const ids = new Set(categories.map((c) => c.id))
  const children = new Map<string, LedgerCategory[]>()
  for (const category of categories) {
    const key = category.parentId && ids.has(category.parentId) ? category.parentId : ""
    children.set(key, [...(children.get(key) ?? []), category])
  }
  children.forEach((list) => list.sort((a, b) => a.name.localeCompare(b.name)))

  const result: CategoryTreeEntry[] = []
  const seen = new Set<string>()
  const visit = (category: LedgerCategory, depth: number, path: string[]) => {
    if (seen.has(category.id)) return
    seen.add(category.id)
    const entry = { category, depth, path: [...path, category.name] }
    result.push(entry)
    for (const child of children.get(category.id) ?? []) visit(child, depth + 1, entry.path)
  }
  for (const root of children.get("") ?? []) visit(root, 0, [])
  for (const category of categories) visit(category, 0, [])
  return result
}

// descendantIds returns the category and everything below it.
export function descendantIds(categories: LedgerCategory[], id: string): Set<string> {
  const result = new Set([id])
  let grew = true
  while (grew) {
    grew = false
    for (const category of categories) {
      if (category.parentId && result.has(category.parentId) && !result.has(category.id)) {
        result.add(category.id)
        grew = true
      }
    }
  }
  return result
}

export const categoryPathSeparator = " › "
