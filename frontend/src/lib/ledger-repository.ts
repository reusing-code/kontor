import type {
  LedgerAccount,
  LedgerAccountInput,
  LedgerCategory,
  LedgerCategoryInput,
  LedgerCommitRequest,
  LedgerCommitResult,
  LedgerImportBatch,
  LedgerPreviewResult,
  LedgerReviewInput,
  LedgerReviewResult,
  LedgerSourceType,
  LedgerTransaction,
  LedgerTransactionDetailsInput,
  LedgerTransactionsPage,
} from "@/types/ledger"
import { del, get, post, postForm, put } from "./api"

export async function getLedgerAccounts(): Promise<LedgerAccount[]> {
  return get<LedgerAccount[]>("/ledger/accounts")
}

export async function getLedgerAccountById(id: string): Promise<LedgerAccount> {
  return get<LedgerAccount>(`/ledger/accounts/${id}`)
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
