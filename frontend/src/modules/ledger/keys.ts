import type { LedgerAnalysisRange, LedgerEmailLinkEntityType } from "@/modules/ledger/types"

const all = ["ledger"] as const
const rangeKey = (range: LedgerAnalysisRange) => ({ from: range.from ?? "", to: range.to ?? "" })

export const ledgerKeys = {
  all,
  accounts: () => [...all, "accounts"] as const,
  account: (id: string) => [...all, "accounts", id] as const,
  accountDeleteSummary: (id: string) => [...all, "accounts", id, "delete-summary"] as const,
  accountTransactions: (id: string) => [...all, "accounts", id, "transactions"] as const,
  accountTransactionsPage: (id: string, limit: number, offset: number) =>
    [...all, "accounts", id, "transactions", { limit, offset }] as const,
  // the cross-account list; refreshed together with every account's list
  transactionList: () => [...all, "transaction-list"] as const,
  transactionListPage: (accountIds: readonly string[], limit: number, offset: number) =>
    [...all, "transaction-list", { accountIds: [...accountIds], limit, offset }] as const,
  transactions: () => [...all, "transactions"] as const,
  transaction: (id: string) => [...all, "transactions", id] as const,
  transferCandidates: (id: string) => [...all, "transactions", id, "transfer-candidates"] as const,
  imports: () => [...all, "imports"] as const,
  categories: () => [...all, "categories"] as const,
  review: () => [...all, "review"] as const,
  reviewQueue: (limit: number | "all", cursor?: string) => [...all, "review", { limit, cursor: cursor ?? "" }] as const,
  reviewCount: () => [...all, "review", "count"] as const,
  suggestionStatus: () => [...all, "category-suggestions", "status"] as const,
  analysis: () => [...all, "analysis"] as const,
  spending: (range: LedgerAnalysisRange, parentCategoryId?: string) =>
    [...all, "analysis", "spending", { ...rangeKey(range), parentCategoryId: parentCategoryId ?? "" }] as const,
  cashFlow: (range: LedgerAnalysisRange) => [...all, "analysis", "cash-flow", rangeKey(range)] as const,
  ranking: (range: LedgerAnalysisRange) => [...all, "analysis", "ranking", rangeKey(range)] as const,
  netWorth: (range: LedgerAnalysisRange) => [...all, "analysis", "net-worth", rangeKey(range)] as const,
  categoryBreakdown: (range: LedgerAnalysisRange) => [...all, "analysis", "category-breakdown", rangeKey(range)] as const,
  emailAccounts: () => [...all, "email-accounts"] as const,
  emailScanStatus: (id: string) => [...all, "email-scan", id] as const,
  emailOrders: () => [...all, "email-orders"] as const,
  emailOrderList: (emailAccountId?: string, status?: string) =>
    [...all, "email-orders", { emailAccountId: emailAccountId ?? "", status: status ?? "" }] as const,
  emailOrder: (id: string) => [...all, "email-orders", id] as const,
  emailOrderCandidates: (id: string, afterDays?: number) =>
    [...all, "email-orders", id, "candidates", { afterDays: afterDays ?? 0 }] as const,
  emailLinks: (entityType: LedgerEmailLinkEntityType, entityId: string) =>
    [...all, "email-links", entityType, entityId] as const,
  bankConnections: () => [...all, "bank", "connections"] as const,
  bankAspsps: (country: string) => [...all, "bank", "aspsps", country] as const,
}
