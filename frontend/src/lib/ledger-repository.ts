import type {
  LedgerAccount,
  LedgerAccountInput,
  LedgerCommitRequest,
  LedgerCommitResult,
  LedgerImportBatch,
  LedgerPreviewResult,
  LedgerSourceType,
  LedgerTransactionsPage,
} from "@/types/ledger"
import { get, post, postForm } from "./api"

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
