import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  commitLedgerImport,
  createLedgerCategory,
  createLedgerEmailAccount,
  deleteLedgerCategory,
  deleteLedgerEmailAccount,
  getLedgerAccountById,
  getLedgerAccounts,
  getLedgerCategories,
  getLedgerEmailAccounts,
  getLedgerEmailImporters,
  getLedgerEmailOrderById,
  getLedgerEmailOrders,
  getLedgerImports,
  getLedgerReviewQueue,
  getLedgerTransactionById,
  getLedgerTransferCandidates,
  getLedgerTransactions,
  linkLedgerEmailOrder,
  linkLedgerTransfer,
  previewLedgerImport,
  rejectLedgerEmailOrder,
  reviewLedgerTransaction,
  scanLedgerEmailAccount,
  unlinkLedgerTransfer,
  updateLedgerTransactionDetails,
  updateLedgerCategory,
  updateLedgerEmailAccount,
} from "@/lib/ledger-repository"
import type {
  LedgerCategoryInput,
  LedgerCommitRequest,
  LedgerEmailAccountInput,
  LedgerEmailOrderLinkInput,
  LedgerReviewInput,
  LedgerSourceType,
  LedgerTransferLinkInput,
  LedgerTransactionDetailsInput,
} from "@/types/ledger"

export const ledgerAccountsKey = ["ledger", "accounts"] as const
export const ledgerEmailAccountsKey = ["ledger", "email-accounts"] as const
export const ledgerImportsKey = ["ledger", "imports"] as const
export const ledgerCategoriesKey = ["ledger", "categories"] as const
export const ledgerEmailImportersKey = ["ledger", "email-importers"] as const
export const ledgerReviewQueueKey = (limit: number, cursor?: string) => ["ledger", "review", { limit, cursor: cursor ?? "" }] as const
const ledgerAccountKey = (accountId: string) => ["ledger", "accounts", accountId] as const
const ledgerTransactionKey = (transactionId: string) => ["ledger", "transactions", transactionId] as const
const ledgerTransferCandidatesKey = (transactionId: string) => ["ledger", "transactions", transactionId, "transfer-candidates"] as const
const ledgerTransactionsKey = (accountId: string, limit: number, cursor?: string) => ["ledger", "accounts", accountId, "transactions", { limit, cursor: cursor ?? "" }] as const
export const ledgerEmailOrdersKey = (emailAccountId?: string, status?: string) => ["ledger", "email-orders", { emailAccountId: emailAccountId ?? "", status: status ?? "" }] as const
const ledgerEmailOrderKey = (id: string) => ["ledger", "email-orders", id] as const

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

export function useLedgerEmailAccounts() {
  return useQuery({
    queryKey: ledgerEmailAccountsKey,
    queryFn: getLedgerEmailAccounts,
  })
}

export function useLedgerEmailOrders(emailAccountId?: string, status?: string) {
  return useQuery({
    queryKey: ledgerEmailOrdersKey(emailAccountId, status),
    queryFn: () => getLedgerEmailOrders(emailAccountId, status),
  })
}

export function useLedgerEmailOrder(id: string) {
  return useQuery({
    queryKey: ledgerEmailOrderKey(id),
    queryFn: () => getLedgerEmailOrderById(id),
    enabled: Boolean(id),
  })
}

export function useLedgerEmailImporters() {
  return useQuery({
    queryKey: ledgerEmailImportersKey,
    queryFn: getLedgerEmailImporters,
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
    enabled: Boolean(transactionId),
  })
}

export function useLedgerTransferCandidates(transactionId: string) {
  return useQuery({
    queryKey: ledgerTransferCandidatesKey(transactionId),
    queryFn: () => getLedgerTransferCandidates(transactionId),
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
      qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(transaction.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", transaction.accountId, "transactions"] })
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["purchases"] })
      qc.invalidateQueries({ queryKey: ["vehicles"] })
    },
  })
}

export function useLinkLedgerTransfer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerTransferLinkInput }) => linkLedgerTransfer(id, data),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.transaction.id) })
      qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(result.transaction.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.transaction.accountId, "transactions"] })
      if (result.pairedTransaction) {
        qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.pairedTransaction.id) })
        qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(result.pairedTransaction.id) })
        qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.pairedTransaction.accountId, "transactions"] })
      }
    },
  })
}

export function useUnlinkLedgerTransfer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unlinkLedgerTransfer(id),
    onSuccess: (result) => {
      qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.transaction.id) })
      qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(result.transaction.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.transaction.accountId, "transactions"] })
      if (result.pairedTransaction) {
        qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.pairedTransaction.id) })
        qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(result.pairedTransaction.id) })
        qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.pairedTransaction.accountId, "transactions"] })
      }
    },
  })
}

export function useCreateLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerEmailAccountInput) => createLedgerEmailAccount(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
    },
  })
}

export function useUpdateLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailAccountInput }) => updateLedgerEmailAccount(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
    },
  })
}

export function useDeleteLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerEmailAccount(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
      qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] })
    },
  })
}

export function useScanLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, files }: { id: string; files: File[] }) => scanLedgerEmailAccount(id, files),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
      qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] })
      qc.invalidateQueries({ queryKey: ["ledger", "transactions"] })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts"] })
    },
  })
}

export function useLinkLedgerEmailOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailOrderLinkInput }) => linkLedgerEmailOrder(id, data),
    onSuccess: (order) => {
      qc.invalidateQueries({ queryKey: ledgerEmailOrderKey(order.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] })
      qc.invalidateQueries({ queryKey: ["ledger", "transactions"] })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts"] })
    },
  })
}

export function useRejectLedgerEmailOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => rejectLedgerEmailOrder(id),
    onSuccess: (order) => {
      qc.invalidateQueries({ queryKey: ledgerEmailOrderKey(order.id) })
      qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] })
      qc.invalidateQueries({ queryKey: ["ledger", "transactions"] })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts"] })
    },
  })
}
