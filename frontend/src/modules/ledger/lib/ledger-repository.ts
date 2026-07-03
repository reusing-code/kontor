import type {
  LedgerAccount,
  LedgerAccountInput,
  LedgerCategory,
  LedgerCategoryInput,
  LedgerEmailAccount,
  LedgerEmailAccountInput,
  LedgerEmailImporterInfo,
  LedgerEmailOrder,
  LedgerEmailOrderLinkInput,
  LedgerEmailScanResult,
  LedgerCommitRequest,
  LedgerCommitResult,
  LedgerImportBatch,
  LedgerPreviewResult,
  LedgerReviewInput,
  LedgerReviewResult,
  LedgerSourceType,
  LedgerTransferCandidatesResult,
  LedgerTransferLinkInput,
  LedgerTransferLinkResult,
  LedgerTransaction,
  LedgerTransactionDetailsInput,
  LedgerTransactionsPage,
} from "@/modules/ledger/types"
import { del, delJson, get, post, postForm, put } from "@/lib/api"

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

export async function getLedgerEmailImporters(): Promise<LedgerEmailImporterInfo[]> {
  return get<LedgerEmailImporterInfo[]>("/ledger/email-importers")
}

export async function linkLedgerEmailOrder(id: string, data: LedgerEmailOrderLinkInput): Promise<LedgerEmailOrder> {
  return post<LedgerEmailOrder>(`/ledger/email-orders/${id}/link`, data)
}

export async function rejectLedgerEmailOrder(id: string): Promise<LedgerEmailOrder> {
  return post<LedgerEmailOrder>(`/ledger/email-orders/${id}/reject`, {})
}

export async function scanLedgerEmailAccount(id: string, files: File[]): Promise<LedgerEmailScanResult> {
  const form = new FormData()
  for (const file of files) {
    form.append("files", file)
  }
  return postForm<LedgerEmailScanResult>(`/ledger/email-accounts/${id}/scan`, form)
}

export async function getLedgerImports(): Promise<LedgerImportBatch[]> {
  return get<LedgerImportBatch[]>("/ledger/imports")
}

export async function getLedgerTransactions(accountId: string, limit: number = 100, cursor?: string): Promise<LedgerTransactionsPage> {
  const search = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    search.set("cursor", cursor)
  }
  return get<LedgerTransactionsPage>(`/ledger/accounts/${accountId}/transactions?${search.toString()}`)
}

export async function getLedgerReviewQueue(limit: number = 100, cursor?: string): Promise<LedgerTransactionsPage> {
  const search = new URLSearchParams({ limit: String(limit) })
  if (cursor) {
    search.set("cursor", cursor)
  }
  return get<LedgerTransactionsPage>(`/ledger/transactions?${search.toString()}`)
}

export async function getLedgerTransactionById(id: string): Promise<LedgerTransaction> {
  return get<LedgerTransaction>(`/ledger/transactions/${id}`)
}

export async function updateLedgerTransactionDetails(id: string, data: LedgerTransactionDetailsInput): Promise<LedgerTransaction> {
  return put<LedgerTransaction>(`/ledger/transactions/${id}`, data)
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

export async function getLedgerCategories(): Promise<LedgerCategory[]> {
  return get<LedgerCategory[]>("/ledger/categories")
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
  sourceType: LedgerSourceType
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
