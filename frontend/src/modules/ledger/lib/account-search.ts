// Search params of /ledger/accounts/$accountId: the 1-based page of the
// transaction list, left out for the first page.
export interface LedgerAccountSearch {
  page?: number
}

export function parseLedgerAccountSearch(search: Record<string, unknown>): LedgerAccountSearch {
  const page = typeof search.page === "string" ? Number(search.page) : search.page
  return typeof page === "number" && Number.isInteger(page) && page > 1 ? { page } : {}
}

export function ledgerAccountSearch(page: number): LedgerAccountSearch {
  return page > 1 ? { page } : {}
}
