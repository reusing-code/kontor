import { useEffect, useRef } from "react"
import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import {
  addLedgerTransactionReference,
  commitLedgerImport,
  createLedgerAccount,
  createLedgerCategory,
  createLedgerEmailAccount,
  deleteLedgerAccount,
  deleteLedgerCategory,
  getLedgerAccountDeleteSummary,
  updateLedgerAccount,
  setLedgerAccountBalance,
  deleteLedgerAccountBalance,
  deleteLedgerEmailAccount,
  getLedgerAccountById,
  getLedgerAccounts,
  getLedgerCategories,
  getLedgerCategorySuggestionStatus,
  getLedgerEmailAccounts,
  getLedgerEmailOrderById,
  getLedgerEmailOrderCandidates,
  getLedgerEmailOrders,
  getLedgerImports,
  getLedgerReviewQueue,
  getLedgerReviewQueueAll,
  getLedgerSpendingAnalysis,
  getLedgerCashFlowAnalysis,
  getLedgerRankingAnalysis,
  getLedgerNetWorthAnalysis,
  getLedgerTransactionById,
  getLedgerTransferCandidates,
  getLedgerTransactions,
  linkLedgerEmailOrder,
  linkLedgerTransfer,
  previewLedgerImport,
  rejectLedgerEmailOrder,
  reviewLedgerTransaction,
  reviewLedgerTransactionsBatch,
  runLedgerCategorySuggestions,
  runLedgerRecurringDetection,
  setLedgerTransactionSplits,
  getLedgerEmailScanStatus,
  startLedgerEmailScan,
  runLedgerEmailDrafts,
  getLedgerEmailLinks,
  uploadLedgerEmailMessages,
  testLedgerEmailAccount,
  listLedgerEmailFolders,
  type LedgerEmailFoldersRequest,
  unlinkLedgerTransfer,
  updateLedgerTransactionDetails,
  updateLedgerCategory,
  updateLedgerEmailAccount,
} from "@/modules/ledger/lib/ledger-repository"
import type {
  LedgerAnalysisRange,
  LedgerBatchReviewItem,
  LedgerAccountInput,
  LedgerAccountUpdateInput,
  LedgerManualBalanceInput,
  LedgerCategoryInput,
  LedgerCommitRequest,
  LedgerEmailAccountInput,
  LedgerEmailLinkEntityType,
  LedgerEmailOrderLinkInput,
  LedgerEmailScanStatus,
  LedgerReviewInput,
  LedgerSourceType,
  LedgerTransferLinkInput,
  LedgerTransferLinkResult,
  LedgerTransaction,
  LedgerTransactionDetailsInput,
  LedgerTransactionReference,
  LedgerTransactionSplit,
} from "@/modules/ledger/types"

export const ledgerAccountsKey = ["ledger", "accounts"] as const
export const ledgerEmailAccountsKey = ["ledger", "email-accounts"] as const
export const ledgerEmailScanStatusKey = (id: string) => ["ledger", "email-scan", id] as const
export const ledgerImportsKey = ["ledger", "imports"] as const
export const ledgerCategoriesKey = ["ledger", "categories"] as const
export const ledgerAnalysisKey = ["ledger", "analysis"] as const
export const ledgerReviewQueueKey = (limit: number | "all", cursor?: string) => ["ledger", "review", { limit, cursor: cursor ?? "" }] as const
export const ledgerSuggestionStatusKey = ["ledger", "category-suggestions", "status"] as const
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

export function useUpdateLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerAccountUpdateInput }) => updateLedgerAccount(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerAccountsKey }),
  })
}

// A balance changes the account, the account list and every page balance of
// the account's transactions, which all live under ["ledger","accounts"].
export function useSetLedgerAccountBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerManualBalanceInput }) => setLedgerAccountBalance(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerAccountsKey }),
  })
}

export function useDeleteLedgerAccountBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, asOf }: { id: string; asOf: string }) => deleteLedgerAccountBalance(id, asOf),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerAccountsKey }),
  })
}

export function useLedgerAccountDeleteSummary(accountId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...ledgerAccountKey(accountId), "delete-summary"],
    queryFn: () => getLedgerAccountDeleteSummary(accountId),
    enabled,
    staleTime: 0,
  })
}

// Deleting an account removes transactions, imports and links in other
// modules and unmaps bank accounts, so every query is refreshed.
export function useDeleteLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerAccount(id),
    onSuccess: (_, id) => {
      qc.removeQueries({ queryKey: ledgerAccountKey(id) })
      return qc.invalidateQueries()
    },
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

// The analyses keep the previous data on screen while another range loads.
export function useLedgerSpendingAnalysis(range: LedgerAnalysisRange, parentCategoryId?: string) {
  return useQuery({
    queryKey: [...ledgerAnalysisKey, "spending", { from: range.from ?? "", to: range.to ?? "", parentCategoryId: parentCategoryId ?? "" }],
    queryFn: () => getLedgerSpendingAnalysis(range, parentCategoryId),
    placeholderData: keepPreviousData,
  })
}

const analysisRangeKey = (range: LedgerAnalysisRange) => ({ from: range.from ?? "", to: range.to ?? "" })

export function useLedgerCashFlowAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: [...ledgerAnalysisKey, "cash-flow", analysisRangeKey(range)],
    queryFn: () => getLedgerCashFlowAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerRankingAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: [...ledgerAnalysisKey, "ranking", analysisRangeKey(range)],
    queryFn: () => getLedgerRankingAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerNetWorthAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: [...ledgerAnalysisKey, "net-worth", analysisRangeKey(range)],
    queryFn: () => getLedgerNetWorthAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerReviewQueue(limit: number = 100, cursor?: string) {
  return useQuery({
    queryKey: ledgerReviewQueueKey(limit, cursor),
    queryFn: () => getLedgerReviewQueue(limit, cursor),
  })
}

// The review page loads the whole queue and groups and pages it itself.
export function useLedgerReviewQueueAll() {
  return useQuery({
    queryKey: ledgerReviewQueueKey("all"),
    queryFn: getLedgerReviewQueueAll,
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
        qc.invalidateQueries({ queryKey: ledgerSuggestionStatusKey }),
        // the backend runs the recurring-payment detection after a commit
        qc.invalidateQueries({ queryKey: ["drafts"] }),
      ])
    },
  })
}

export function useRunLedgerRecurringDetection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: runLedgerRecurringDetection,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["drafts"] }),
  })
}

// Polls while a suggestion run (manual or the background run after an import)
// is going and refreshes the review queue once it ends.
export function useLedgerSuggestionStatus(enabled: boolean) {
  const qc = useQueryClient()
  const { data } = useQuery({
    queryKey: ledgerSuggestionStatusKey,
    queryFn: getLedgerCategorySuggestionStatus,
    enabled,
    refetchInterval: (query) => (query.state.data?.running ? 5000 : false),
  })
  const running = enabled && (data?.running ?? false)
  const wasRunning = useRef(running)
  useEffect(() => {
    if (wasRunning.current && !running) {
      void qc.invalidateQueries({ queryKey: ["ledger", "review"] })
    }
    wasRunning.current = running
  }, [running, qc])
  return running
}

export function useRunLedgerCategorySuggestions() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: runLedgerCategorySuggestions,
    onSettled: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["ledger", "review"] }),
        qc.invalidateQueries({ queryKey: ledgerSuggestionStatusKey }),
      ])
    },
  })
}

export function useReviewLedgerTransactionsBatch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (items: LedgerBatchReviewItem[]) => reviewLedgerTransactionsBatch(items),
    // also after a failed chunk: the chunks before it were stored
    onSettled: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerCategoriesKey }),
        qc.invalidateQueries({ queryKey: ["ledger", "review"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "transactions"] }),
        qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
      ])
    },
  })
}

export function useCreateLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerAccountInput) => createLedgerAccount(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerAccountsKey })
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

function invalidateTransactionDetails(qc: QueryClient, transaction: LedgerTransaction) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ledgerTransactionKey(transaction.id) }),
    qc.invalidateQueries({ queryKey: ledgerTransferCandidatesKey(transaction.id) }),
    qc.invalidateQueries({ queryKey: ["ledger", "accounts", transaction.accountId, "transactions"] }),
    qc.invalidateQueries({ queryKey: ["contracts"] }),
    qc.invalidateQueries({ queryKey: ["purchases"] }),
    qc.invalidateQueries({ queryKey: ["vehicles"] }),
  ])
}

export function useUpdateLedgerTransactionDetails() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerTransactionDetailsInput }) => updateLedgerTransactionDetails(id, data),
    onSuccess: (transaction) => invalidateTransactionDetails(qc, transaction),
  })
}

export function useSetLedgerTransactionSplits() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, splits }: { id: string; splits: LedgerTransactionSplit[] }) => setLedgerTransactionSplits(id, splits),
    onSuccess: (transaction) =>
      Promise.all([invalidateTransactionDetails(qc, transaction), qc.invalidateQueries({ queryKey: ["ledger", "review"] })]),
  })
}

// Links one target right away (the card's Save would send the whole list).
export function useAddLedgerTransactionReference() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reference }: { id: string; reference: LedgerTransactionReference }) =>
      addLedgerTransactionReference(id, reference),
    onSuccess: (transaction) => invalidateTransactionDetails(qc, transaction),
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

function invalidateAfterEmailScan(qc: QueryClient) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ledgerEmailAccountsKey }),
    qc.invalidateQueries({ queryKey: ["ledger", "email-orders"] }),
    qc.invalidateQueries({ queryKey: ["ledger", "transactions"] }),
    qc.invalidateQueries({ queryKey: ["ledger", "accounts"] }),
  ])
}

// Polls while a scan of the account runs (started here, by the scheduler or
// before a page reload) and reports it once when it ends.
export function useLedgerEmailScanStatus(id: string, onFinished: (status: LedgerEmailScanStatus) => void) {
  const qc = useQueryClient()
  const { data } = useQuery({
    queryKey: ledgerEmailScanStatusKey(id),
    queryFn: () => getLedgerEmailScanStatus(id),
    refetchInterval: (query) => (query.state.data?.state === "running" ? 2000 : false),
  })
  const running = data?.state === "running"
  const wasRunning = useRef(running)
  const finished = useRef(onFinished)
  useEffect(() => {
    finished.current = onFinished
  })
  useEffect(() => {
    if (wasRunning.current && !running && data) {
      void invalidateAfterEmailScan(qc)
      finished.current(data)
    }
    wasRunning.current = running
  }, [running, data, qc])
  return data
}

export function useStartLedgerEmailScan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => startLedgerEmailScan(id),
    onSuccess: (status, id) => {
      qc.setQueryData(ledgerEmailScanStatusKey(id), status)
    },
    // A 409 means a scan is already running: pick it up.
    onError: (_error, id) => qc.invalidateQueries({ queryKey: ledgerEmailScanStatusKey(id) }),
  })
}

export function useUploadLedgerEmailMessages() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, files }: { id: string; files: File[] }) => uploadLedgerEmailMessages(id, files),
    onSuccess: () => invalidateAfterEmailScan(qc),
  })
}

export function useTestLedgerEmailAccount() {
  return useMutation({
    mutationFn: (id: string) => testLedgerEmailAccount(id),
  })
}

// Checks the drafts folders of all accounts; new drafts show up in the inbox.
export function useRunLedgerEmailDrafts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => runLedgerEmailDrafts(),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["drafts"] }),
  })
}

export function useLedgerEmailLinks(entityType: LedgerEmailLinkEntityType, entityId: string, enabled: boolean) {
  return useQuery({
    queryKey: ["ledger", "email-links", entityType, entityId],
    queryFn: () => getLedgerEmailLinks(entityType, entityId),
    enabled,
  })
}

export function useLedgerEmailFolders() {
  return useMutation({
    mutationFn: (data: LedgerEmailFoldersRequest) => listLedgerEmailFolders(data),
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
