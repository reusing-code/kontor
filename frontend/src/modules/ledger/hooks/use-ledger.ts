import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
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
  getLedgerEmailOrderById,
  getLedgerEmailOrderCandidates,
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
  testLedgerEmailAccount,
  unlinkLedgerTransfer,
  updateLedgerTransactionDetails,
  updateLedgerCategory,
  updateLedgerEmailAccount,
} from "@/modules/ledger/lib/ledger-repository"
import type {
  LedgerCategoryInput,
  LedgerCommitRequest,
  LedgerEmailAccountInput,
  LedgerEmailOrderLinkInput,
  LedgerReviewInput,
  LedgerSourceType,
  LedgerTransferLinkInput,
  LedgerTransferLinkResult,
  LedgerTransactionDetailsInput,
} from "@/modules/ledger/types"

export const ledgerAccountsKey = ["ledger", "accounts"] as const
export const ledgerEmailAccountsKey = ["ledger", "email-accounts"] as const
export const ledgerImportsKey = ["ledger", "imports"] as const
export const ledgerCategoriesKey = ["ledger", "categories"] as const
export const ledgerReviewQueueKey = (limit: number, cursor?: string) => ["ledger", "review", { limit, cursor: cursor ?? "" }] as const
const ledgerAccountKey = (accountId: string) => ["ledger", "accounts", accountId] as const
const ledgerTransactionKey = (transactionId: string) => ["ledger", "transactions", transactionId] as const
const ledgerTransferCandidatesKey = (transactionId: string) => ["ledger", "transactions", transactionId, "transfer-candidates"] as const
const ledgerTransactionsKey = (accountId: string, limit: number, cursor?: string) => ["ledger", "accounts", accountId, "transactions", { limit, cursor: cursor ?? "" }] as const
export const ledgerEmailOrdersKey = (emailAccountId?: string, status?: string) => ["ledger", "email-orders", { emailAccountId: emailAccountId ?? "", status: status ?? "" }] as const
const ledgerEmailOrderKey = (id: string) => ["ledger", "email-orders", id] as const
const ledgerEmailOrderCandidatesKey = (id: string, afterDays?: number) => ["ledger", "email-orders", id, "candidates", { afterDays: afterDays ?? 0 }] as const

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

export function useLedgerEmailOrderCandidates(id: string, afterDays?: number) {
  return useQuery({
    queryKey: ledgerEmailOrderCandidatesKey(id, afterDays),
    queryFn: () => getLedgerEmailOrderCandidates(id, afterDays),
    enabled: Boolean(id),
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
    enabled: accountId !== "",
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
    onSuccess: async (result) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerAccountsKey }),
        qc.invalidateQueries({ queryKey: ledgerImportsKey }),
        qc.invalidateQueries({ queryKey: ledgerCategoriesKey }),
        qc.invalidateQueries({ queryKey: ledgerAccountKey(result.accountId) }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.accountId, "transactions"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "review"] }),
      ])
    },
  })
}

export function useCreateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerCategoryInput) => createLedgerCategory(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
    },
  })
}

export function useUpdateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerCategoryInput }) => updateLedgerCategory(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerCategoriesKey })
    },
  })
}

export function useDeleteLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerCategory(id),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerCategoriesKey }),
        qc.invalidateQueries({ queryKey: ["ledger", "review"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
      ])
    },
  })
}

export function useReviewLedgerTransaction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerReviewInput }) => reviewLedgerTransaction(id, data),
    onSuccess: async (result) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerCategoriesKey }),
        qc.invalidateQueries({ queryKey: ["ledger", "review"] }),
        qc.invalidateQueries({ queryKey: ledgerTransactionKey(result.transaction.id) }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.transaction.accountId, "transactions"] }),
      ])
    },
  })
}

export function useUpdateLedgerTransactionDetails() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerTransactionDetailsInput }) => updateLedgerTransactionDetails(id, data),
    onSuccess: async (transaction) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerTransactionKey(transaction.id) }),
        qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(transaction.id) }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts", transaction.accountId, "transactions"] }),
        qc.invalidateQueries({ queryKey: ["contracts"] }),
        qc.invalidateQueries({ queryKey: ["purchases"] }),
        qc.invalidateQueries({ queryKey: ["vehicles"] }),
      ])
    },
  })
}

function invalidateTransferPair(qc: QueryClient, result: LedgerTransferLinkResult) {
  const transactions = result.pairedTransaction ? [result.transaction, result.pairedTransaction] : [result.transaction]
  return Promise.all(
    transactions.flatMap((txn) => [
      qc.invalidateQueries({ queryKey: ledgerTransactionKey(txn.id) }),
      qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(txn.id) }),
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", txn.accountId, "transactions"] }),
    ]),
  )
}

export function useLinkLedgerTransfer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerTransferLinkInput }) => linkLedgerTransfer(id, data),
    onSuccess: (result) => invalidateTransferPair(qc, result),
  })
}

export function useUnlinkLedgerTransfer() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => unlinkLedgerTransfer(id),
    onSuccess: (result) => invalidateTransferPair(qc, result),
  })
}

export function useCreateLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerEmailAccountInput) => createLedgerEmailAccount(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
    },
  })
}

export function useUpdateLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailAccountInput }) => updateLedgerEmailAccount(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey })
    },
  })
}

export function useDeleteLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerEmailAccount(id),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey }),
        qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] }),
      ])
    },
  })
}

export function useScanLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, files }: { id: string; files: File[] }) => scanLedgerEmailAccount(id, files),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey }),
        qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "transactions"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
      ])
    },
  })
}

export function useTestLedgerEmailAccount() {
  return useMutation({
    mutationFn: (id: string) => testLedgerEmailAccount(id),
  })
}

export function useLinkLedgerEmailOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailOrderLinkInput }) => linkLedgerEmailOrder(id, data),
    onSuccess: async (order) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerEmailOrderKey(order.id) }),
        qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "transactions"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
      ])
    },
  })
}

export function useRejectLedgerEmailOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => rejectLedgerEmailOrder(id),
    onSuccess: async (order) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerEmailOrderKey(order.id) }),
        qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "transactions"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
      ])
    },
  })
}
