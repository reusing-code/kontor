import type {
  LedgerBatchReviewItem,
  LedgerBatchReviewResult,
  LedgerReviewQueuePage,
  LedgerRecurringRunResult,
  LedgerSuggestionRunResult,
  LedgerSuggestionStatus,
  LedgerAccount,
  LedgerAccountDeleteSummary,
  LedgerAccountInput,
  LedgerAccountUpdateInput,
  LedgerAnalysisRange,
  LedgerCashFlowAnalysis,
  LedgerCategory,
  LedgerCategoryInput,
  LedgerEmailAccount,
  LedgerEmailAccountInput,
  LedgerEmailDraftRunResult,
  LedgerEmailLink,
  LedgerEmailLinkEntityType,
  LedgerEmailOrder,
  LedgerEmailOrderCandidatesResult,
  LedgerEmailOrderLinkInput,
  LedgerEmailScanResult,
  LedgerEmailScanStatus,
  LedgerCommitRequest,
  LedgerCommitResult,
  LedgerImportBatch,
  LedgerManualBalanceInput,
  LedgerNetWorthAnalysis,
  LedgerPreviewResult,
  LedgerRankingAnalysis,
  LedgerReviewInput,
  LedgerReviewResult,
  LedgerFileSourceType,
  LedgerSpendingAnalysis,
  LedgerTransferCandidatesResult,
  LedgerTransferLinkInput,
  LedgerTransferLinkResult,
  LedgerTransaction,
  LedgerTransactionDetailsInput,
  LedgerTransactionReference,
  LedgerTransactionSplit,
  LedgerTransactionList,
  LedgerTransactionsPage,
} from "@/modules/ledger/types"
import { del, delJson, get, post, postForm, put } from "@/lib/api"
import { analysisQuery } from "@/modules/ledger/lib/analysis-range"

export async function getLedgerAccounts(): Promise<LedgerAccount[]> {
  return get<LedgerAccount[]>("/ledger/accounts")
}

export async function getLedgerAccountById(id: string): Promise<LedgerAccount> {
  return get<LedgerAccount>(`/ledger/accounts/${id}`)
}

export async function getLedgerEmailAccounts(): Promise<LedgerEmailAccount[]> {
  return get<LedgerEmailAccount[]>("/ledger/email-accounts")
}

export async function createLedgerEmailAccount(data: LedgerEmailAccountInput): Promise<LedgerEmailAccount> {
  return post<LedgerEmailAccount>("/ledger/email-accounts", data)
}

export async function updateLedgerEmailAccount(id: string, data: LedgerEmailAccountInput): Promise<LedgerEmailAccount> {
  return put<LedgerEmailAccount>(`/ledger/email-accounts/${id}`, data)
}

export async function deleteLedgerEmailAccount(id: string): Promise<void> {
  return del(`/ledger/email-accounts/${id}`)
}

export async function testLedgerEmailAccount(id: string): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>(`/ledger/email-accounts/${id}/test`, {})
}

export interface LedgerEmailFoldersRequest {
  accountId?: string
  imapHost: string
  imapPort: number
  username: string
  password?: string
  useTls: boolean
}

export async function listLedgerEmailFolders(data: LedgerEmailFoldersRequest): Promise<string[]> {
  const res = await post<{ folders: string[] }>("/ledger/email-accounts/folders", data)
  return res.folders
}

export async function getLedgerEmailOrders(emailAccountId?: string, status?: string): Promise<LedgerEmailOrder[]> {
  const params = new URLSearchParams()
  if (emailAccountId) {
    params.set("emailAccountId", emailAccountId)
  }
  if (status) {
    params.set("status", status)
  }
  const query = params.toString()
  return get<LedgerEmailOrder[]>(`/ledger/email-orders${query ? `?${query}` : ""}`)
}

export async function getLedgerEmailOrderById(id: string): Promise<LedgerEmailOrder> {
  return get<LedgerEmailOrder>(`/ledger/email-orders/${id}`)
}

export async function getLedgerEmailOrderCandidates(id: string, afterDays?: number): Promise<LedgerEmailOrderCandidatesResult> {
  const query = afterDays === undefined ? "" : `?afterDays=${afterDays}`
  return get<LedgerEmailOrderCandidatesResult>(`/ledger/email-orders/${id}/candidates${query}`)
}

export async function linkLedgerEmailOrder(id: string, data: LedgerEmailOrderLinkInput): Promise<LedgerEmailOrder> {
  return post<LedgerEmailOrder>(`/ledger/email-orders/${id}/link`, data)
}

export async function rejectLedgerEmailOrder(id: string): Promise<LedgerEmailOrder> {
  return post<LedgerEmailOrder>(`/ledger/email-orders/${id}/reject`, {})
}

export async function runLedgerEmailDrafts(): Promise<LedgerEmailDraftRunResult> {
  return post<LedgerEmailDraftRunResult>("/ledger/email-drafts/run", {})
}

export async function getLedgerEmailLinks(entityType: LedgerEmailLinkEntityType, entityId: string): Promise<LedgerEmailLink[]> {
  return get<LedgerEmailLink[]>(`/ledger/email-links/${entityType}/${entityId}`)
}

export async function startLedgerEmailScan(id: string): Promise<LedgerEmailScanStatus> {
  return post<LedgerEmailScanStatus>(`/ledger/email-accounts/${id}/scan`, {})
}

export async function getLedgerEmailScanStatus(id: string): Promise<LedgerEmailScanStatus> {
  return get<LedgerEmailScanStatus>(`/ledger/email-accounts/${id}/scan`)
}

export async function uploadLedgerEmailMessages(id: string, files: File[]): Promise<LedgerEmailScanResult> {
  const form = new FormData()
  for (const file of files) {
    form.append("files", file)
  }
  return postForm<LedgerEmailScanResult>(`/ledger/email-accounts/${id}/scan`, form)
}

export async function getLedgerImports(): Promise<LedgerImportBatch[]> {
  return get<LedgerImportBatch[]>("/ledger/imports")
}

export async function getLedgerTransactions(accountId: string, limit: number, offset: number): Promise<LedgerTransactionsPage> {
  const search = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  return get<LedgerTransactionsPage>(`/ledger/accounts/${accountId}/transactions?${search.toString()}`)
}

// The size of the whole queue comes with every page; one item is enough.
export async function getLedgerReviewCount(): Promise<number> {
  const page = await get<LedgerReviewQueuePage>("/ledger/review-queue?limit=1")
  return page.total
}

// getAllLedgerTransactions pages the transactions of all accounts, or of the
// given ones.
export async function getAllLedgerTransactions(accountIds: readonly string[], limit: number, offset: number): Promise<LedgerTransactionList> {
  const search = new URLSearchParams({ limit: String(limit), offset: String(offset) })
  for (const id of accountIds) {
    search.append("accountId", id)
  }
  return get<LedgerTransactionList>(`/ledger/transactions?${search.toString()}`)
}

export async function getLedgerReviewQueueAll(): Promise<LedgerReviewQueuePage> {
  return get<LedgerReviewQueuePage>("/ledger/review-queue?limit=all")
}

// The backend takes at most this many items per review-batch request.
export const ledgerReviewBatchChunkSize = 100

// reviewLedgerTransactionsBatch sends the items in sequential chunks and
// merges the results; a failing request ends the run.
export async function reviewLedgerTransactionsBatch(items: LedgerBatchReviewItem[]): Promise<LedgerBatchReviewResult> {
  const result: LedgerBatchReviewResult = { reviewed: 0, failed: [] }
  for (let start = 0; start < items.length; start += ledgerReviewBatchChunkSize) {
    const chunk = items.slice(start, start + ledgerReviewBatchChunkSize)
    const part = await post<LedgerBatchReviewResult>("/ledger/transactions/review-batch", { items: chunk })
    result.reviewed += part.reviewed
    result.failed.push(...part.failed)
  }
  return result
}

export async function runLedgerCategorySuggestions(): Promise<LedgerSuggestionRunResult> {
  return post<LedgerSuggestionRunResult>("/ledger/category-suggestions/run", {})
}

export async function getLedgerCategorySuggestionStatus(): Promise<LedgerSuggestionStatus> {
  return get<LedgerSuggestionStatus>("/ledger/category-suggestions/status")
}

export async function runLedgerRecurringDetection(): Promise<LedgerRecurringRunResult> {
  return post<LedgerRecurringRunResult>("/ledger/recurring/run", {})
}

export async function getLedgerTransactionById(id: string): Promise<LedgerTransaction> {
  return get<LedgerTransaction>(`/ledger/transactions/${id}`)
}

export async function updateLedgerTransactionDetails(id: string, data: LedgerTransactionDetailsInput): Promise<LedgerTransaction> {
  return put<LedgerTransaction>(`/ledger/transactions/${id}`, data)
}

// Replaces the parts of a transaction; an empty list removes the split.
export async function setLedgerTransactionSplits(id: string, splits: LedgerTransactionSplit[]): Promise<LedgerTransaction> {
  return put<LedgerTransaction>(`/ledger/transactions/${id}/splits`, { splits })
}

export async function addLedgerTransactionReference(id: string, reference: LedgerTransactionReference): Promise<LedgerTransaction> {
  return post<LedgerTransaction>(`/ledger/transactions/${id}/references`, reference)
}

export async function getLedgerTransferCandidates(id: string): Promise<LedgerTransferCandidatesResult> {
  return get<LedgerTransferCandidatesResult>(`/ledger/transactions/${id}/transfer-candidates`)
}

export async function linkLedgerTransfer(id: string, data: LedgerTransferLinkInput): Promise<LedgerTransferLinkResult> {
  return post<LedgerTransferLinkResult>(`/ledger/transactions/${id}/transfer-link`, data)
}

export async function unlinkLedgerTransfer(id: string): Promise<LedgerTransferLinkResult> {
  return delJson<LedgerTransferLinkResult>(`/ledger/transactions/${id}/transfer-link`)
}

export async function updateLedgerAccount(id: string, data: LedgerAccountUpdateInput): Promise<LedgerAccount> {
  return put<LedgerAccount>(`/ledger/accounts/${id}`, data)
}

export async function setLedgerAccountBalance(id: string, data: LedgerManualBalanceInput): Promise<LedgerAccount> {
  return post<LedgerAccount>(`/ledger/accounts/${id}/balance`, data)
}

export async function deleteLedgerAccountBalance(id: string, asOf: string): Promise<LedgerAccount> {
  return delJson<LedgerAccount>(`/ledger/accounts/${id}/balance/${asOf}`)
}

export async function getLedgerAccountDeleteSummary(id: string): Promise<LedgerAccountDeleteSummary> {
  return get<LedgerAccountDeleteSummary>(`/ledger/accounts/${id}/delete-summary`)
}

export async function deleteLedgerAccount(id: string): Promise<void> {
  return del(`/ledger/accounts/${id}`)
}

export async function getLedgerCategories(): Promise<LedgerCategory[]> {
  return get<LedgerCategory[]>("/ledger/categories")
}

export async function createLedgerAccount(data: LedgerAccountInput): Promise<LedgerAccount> {
  return post<LedgerAccount>("/ledger/accounts", data)
}

export async function createLedgerCategory(data: LedgerCategoryInput): Promise<LedgerCategory> {
  return post<LedgerCategory>("/ledger/categories", data)
}

export async function updateLedgerCategory(id: string, data: LedgerCategoryInput): Promise<LedgerCategory> {
  return put<LedgerCategory>(`/ledger/categories/${id}`, data)
}

export async function deleteLedgerCategory(id: string): Promise<void> {
  return del(`/ledger/categories/${id}`)
}

export async function reviewLedgerTransaction(id: string, data: LedgerReviewInput): Promise<LedgerReviewResult> {
  return post<LedgerReviewResult>(`/ledger/transactions/${id}/review`, data)
}

export async function previewLedgerImport(input: {
  file: File
  sourceType: LedgerFileSourceType
  accountId?: string
}): Promise<LedgerPreviewResult> {
  const form = new FormData()
  form.append("file", input.file)
  form.append("sourceType", input.sourceType)
  if (input.accountId) {
    form.append("accountId", input.accountId)
  }
  return postForm<LedgerPreviewResult>("/ledger/imports/preview", form)
}

export async function commitLedgerImport(previewId: string, data: LedgerCommitRequest): Promise<LedgerCommitResult> {
  return post<LedgerCommitResult>(`/ledger/imports/${previewId}/commit`, data)
}

export function defaultLedgerAccountInput(iban?: string, bank?: string): LedgerAccountInput {
  return {
    name: "",
    bank: bank ?? "",
    iban: iban ?? "",
    currency: "EUR",
  }
}

export async function getLedgerSpendingAnalysis(range: LedgerAnalysisRange, parentCategoryId?: string): Promise<LedgerSpendingAnalysis> {
  return get<LedgerSpendingAnalysis>(`/ledger/analysis/spending${analysisQuery(range, { parentCategoryId })}`)
}

export async function getLedgerCashFlowAnalysis(range: LedgerAnalysisRange): Promise<LedgerCashFlowAnalysis> {
  return get<LedgerCashFlowAnalysis>(`/ledger/analysis/cash-flow${analysisQuery(range)}`)
}

export async function getLedgerRankingAnalysis(range: LedgerAnalysisRange): Promise<LedgerRankingAnalysis> {
  return get<LedgerRankingAnalysis>(`/ledger/analysis/ranking${analysisQuery(range)}`)
}

export async function getLedgerNetWorthAnalysis(range: LedgerAnalysisRange): Promise<LedgerNetWorthAnalysis> {
  return get<LedgerNetWorthAnalysis>(`/ledger/analysis/net-worth${analysisQuery(range)}`)
}
