export type PageItem = number | "ellipsis"

// pageItems lists the page buttons of a pager (1-based): the first and the
// last page, the pages next to the current one, and an ellipsis for every gap
// of two or more pages (a gap of one page shows that page instead).
export function pageItems(page: number, pageCount: number, siblings = 1): PageItem[] {
  if (pageCount < 1) return []
  const current = Math.min(Math.max(page, 1), pageCount)
  const from = Math.max(1, current - siblings)
  const to = Math.min(pageCount, current + siblings)
  const shown = new Set([1, pageCount])
  for (let p = from; p <= to; p += 1) shown.add(p)
  const sorted = [...shown].sort((a, b) => a - b)

  return sorted.flatMap((p, i): PageItem[] => {
    const previous = sorted[i - 1]
    if (previous === undefined || p - previous === 1) return [p]
    if (p - previous === 2) return [p - 1, p]
    return ["ellipsis", p]
  })
}

export function pageCountOf(total: number, pageSize: number): number {
  return Math.max(1, Math.ceil(total / pageSize))
}

// parsePageParam reads a 1-based page from a URL search param; the first page
// and anything that is not a page give undefined, so the URL leaves it out.
export function parsePageParam(value: unknown): number | undefined {
  const page = typeof value === "string" ? Number(value) : value
  return typeof page === "number" && Number.isInteger(page) && page > 1 ? page : undefined
}
