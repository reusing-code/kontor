import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  commitLedgerImport,
  createLedgerCategory,
  deleteLedgerCategory,
  getLedgerAccountById,
  getLedgerAccounts,
  getLedgerCategories,
  getLedgerImports,
  getLedgerReviewQueue,
  getLedgerTransactionById,
  getLedgerTransactions,
  previewLedgerImport,
  reviewLedgerTransaction,
  updateLedgerTransactionDetails,
  updateLedgerCategory,
} from "@/lib/ledger-repository"
import type {
  LedgerCategoryInput,
  LedgerCommitRequest,
  LedgerReviewInput,
  LedgerSourceType,
  LedgerTransactionDetailsInput,
} from "@/types/ledger"

export const ledgerAccountsKey = ["ledger", "accounts"] as const
export const ledgerImportsKey = ["ledger", "imports"] as const
export const ledgerCategoriesKey = ["ledger", "categories"] as const
export const ledgerReviewQueueKey = (limit: number, cursor?: string) => ["ledger", "review", { limit, cursor: cursor ?? "" }] as const
const ledgerAccountKey = (accountId: string) => ["ledger", "accounts", accountId] as const
const ledgerTransactionKey = (transactionId: string) => ["ledger", "transactions", transactionId] as const
const ledgerTransactionsKey = (accountId: string, limit: number, cursor?: string) => ["ledger", "accounts", accountId, "transactions", { limit, cursor: cursor ?? "" }] as const

export function useLedgerAccounts() {
  return useQuery({
    queryKey: ledgerAccountsKey,
    queryFn: getLedgerAccounts,
  })
}

export function useLedgerAccount(accountId: string) {
  return useQuery({
    queryKey: ledgerAccountKey(accountId),
    queryFn: () => getLedgerAccountById(accountId),
  })
}

export function useLedgerImports() {
  return useQuery({
    queryKey: ledgerImportsKey,
    queryFn: getLedgerImports,
  })
}

export function useLedgerTransactions(accountId: string, limit: number = 100, cursor?: string) {
  return useQuery({
    queryKey: ledgerTransactionsKey(accountId, limit, cursor),
    queryFn: () => getLedgerTransactions(accountId, limit, cursor),
  })
}

export function useLedgerCategories() {
  return useQuery({
    queryKey: ledgerCategoriesKey,
    queryFn: getLedgerCategories,
  })
}

export function useLedgerReviewQueue(limit: number = 100, cursor?: string) {
  return useQuery({
    queryKey: ledgerReviewQueueKey(limit, cursor),
    queryFn: () => getLedgerReviewQueue(limit, cursor),
  })
}

export function useLedgerTransaction(transactionId: string) {
  return useQuery({
    queryKey: ledgerTransactionKey(transactionId),
    queryFn: () => getLedgerTransactionById(transactionId),
  })
}

export function useLedgerPreviewImport() {
  return useMutation({
    mutationFn: (input: { file: File; sourceType: LedgerSourceType; accountId?: string }) => previewLedgerImport(input),
  })
}

export function useLedgerCommitImport() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ previewId, data }: { previewId: string; data: LedgerCommitRequest }) => commitLedgerImport(previewId, data),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ledgerAccountsKey })
      qc.invalidateQueries({ queryKey: ledgerImportsKey })
      qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
      qc.invalidateQueries({ queryKey: ledgerAccountKey(result.accountId) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.accountId, "transactions"] })
      qc.invalidateQueries({ queryKey: ["ledger", "review"] })
    },
  })
}

export function useCreateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerCategoryInput) => createLedgerCategory(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
    },
  })
}

export function useUpdateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerCategoryInput }) => updateLedgerCategory(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
    },
  })
}

export function useDeleteLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerCategory(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
      qc.invalidateQueries({ queryKey: ["ledger", "review"] })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts"] })
    },
  })
}

export function useReviewLedgerTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerReviewInput }) => reviewLedgerTransaction(id, data),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
      qc.invalidateQueries({ queryKey: ["ledger", "review"] })
      qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.transaction.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.transaction.accountId, "transactions"] })
    },
  })
}

export function useUpdateLedgerTransactionDetails() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerTransactionDetailsInput }) => updateLedgerTransactionDetails(id, data),
    onSuccess: (transaction) => {
      qc.invalidateQueries({ queryKey: ledgerTransactionKey(transaction.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", transaction.accountId, "transactions"] })
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: ["vehicles"] })
    },
  })
}
