import { parsePageParam } from "@/lib/pagination"

export interface AllTransactionsSearch {
  // the selected account IDs; absent = all accounts
  accounts?: string[]
  // 1-based page; absent = the first
  page?: number
}

export function parseAllTransactionsSearch(search: Record<string, unknown>): AllTransactionsSearch {
  const raw = Array.isArray(search.accounts) ? search.accounts : []
  const accounts = [...new Set(raw.filter((id): id is string => typeof id === "string" && id !== ""))]
  const page = parsePageParam(search.page)
  return { ...(accounts.length > 0 ? { accounts } : {}), ...(page ? { page } : {}) }
}

export function allTransactionsSearch(accounts: readonly string[] | undefined, page: number): AllTransactionsSearch {
  return { ...(accounts && accounts.length > 0 ? { accounts: [...accounts] } : {}), ...(page > 1 ? { page } : {}) }
}

// selectedAccountIds keeps the selected IDs that name a known account, so a
// deleted account left in the URL does not empty the list.
export function selectedAccountIds(selected: readonly string[] | undefined, knownIds: readonly string[]): string[] {
  if (!selected) return []
  const known = new Set(knownIds)
  return selected.filter((id) => known.has(id))
}

export function toggleAccount(selected: readonly string[], id: string): string[] {
  return selected.includes(id) ? selected.filter((other) => other !== id) : [...selected, id]
}
