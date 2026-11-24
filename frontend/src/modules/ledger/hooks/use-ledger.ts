import { useEffect, useRef } from "react"
import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import i18n from "@/i18n"
import { ApiError } from "@/lib/api"
import { defaultErrorMessage } from "@/lib/query-client"
import { draftKeys } from "@/lib/query-keys"
import { vehicleKeys } from "@/modules/auto/keys"
import { contractKeys } from "@/modules/contracts/keys"
import { ledgerKeys } from "@/modules/ledger/keys"
import { purchaseKeys } from "@/modules/purchases/keys"
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
  getLedgerReviewCount,
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

export function useLedgerAccounts() {
  return useQuery({
    queryKey: ledgerKeys.accounts(),
    queryFn: getLedgerAccounts,
  })
}

export function useLedgerAccount(accountId: string) {
  return useQuery({
    queryKey: ledgerKeys.account(accountId),
    queryFn: () => getLedgerAccountById(accountId),
  })
}

export function useUpdateLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerAccountUpdateInput }) => updateLedgerAccount(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
  })
}

// A balance changes the account, the account list and every page balance of
// the account's transactions, which all live under ["ledger","accounts"].
export function useSetLedgerAccountBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerManualBalanceInput }) => setLedgerAccountBalance(id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
  })
}

export function useDeleteLedgerAccountBalance() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, asOf }: { id: string; asOf: string }) => deleteLedgerAccountBalance(id, asOf),
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
  })
}

export function useLedgerAccountDeleteSummary(accountId: string, enabled: boolean) {
  return useQuery({
    queryKey: ledgerKeys.accountDeleteSummary(accountId),
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
      qc.removeQueries({ queryKey: ledgerKeys.account(id) })
      return qc.invalidateQueries()
    },
  })
}

export function useLedgerEmailAccounts() {
  return useQuery({
    queryKey: ledgerKeys.emailAccounts(),
    queryFn: getLedgerEmailAccounts,
  })
}

export function useLedgerEmailOrders(emailAccountId?: string, status?: string) {
  return useQuery({
    queryKey: ledgerKeys.emailOrderList(emailAccountId, status),
    queryFn: () => getLedgerEmailOrders(emailAccountId, status),
  })
}

export function useLedgerEmailOrder(id: string) {
  return useQuery({
    queryKey: ledgerKeys.emailOrder(id),
    queryFn: () => getLedgerEmailOrderById(id),
    enabled: Boolean(id),
  })
}

export function useLedgerEmailOrderCandidates(id: string, afterDays?: number) {
  return useQuery({
    queryKey: ledgerKeys.emailOrderCandidates(id, afterDays),
    queryFn: () => getLedgerEmailOrderCandidates(id, afterDays),
    enabled: Boolean(id),
  })
}

export function useLedgerImports() {
  return useQuery({
    queryKey: ledgerKeys.imports(),
    queryFn: getLedgerImports,
  })
}

export function useLedgerTransactions(accountId: string, limit: number = 100, cursor?: string) {
  return useQuery({
    queryKey: ledgerKeys.accountTransactionsPage(accountId, limit, cursor),
    queryFn: () => getLedgerTransactions(accountId, limit, cursor),
    enabled: accountId !== "",
  })
}

export function useLedgerCategories() {
  return useQuery({
    queryKey: ledgerKeys.categories(),
    queryFn: getLedgerCategories,
  })
}

// The analyses keep the previous data on screen while another range loads.
export function useLedgerSpendingAnalysis(range: LedgerAnalysisRange, parentCategoryId?: string) {
  return useQuery({
    queryKey: ledgerKeys.spending(range, parentCategoryId),
    queryFn: () => getLedgerSpendingAnalysis(range, parentCategoryId),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerCashFlowAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.cashFlow(range),
    queryFn: () => getLedgerCashFlowAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerRankingAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.ranking(range),
    queryFn: () => getLedgerRankingAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerNetWorthAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.netWorth(range),
    queryFn: () => getLedgerNetWorthAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerReviewCount() {
  return useQuery({
    queryKey: ledgerKeys.reviewCount(),
    queryFn: getLedgerReviewCount,
  })
}

// The review page loads the whole queue and groups and pages it itself.
export function useLedgerReviewQueueAll() {
  return useQuery({
    queryKey: ledgerKeys.reviewQueue("all"),
    queryFn: getLedgerReviewQueueAll,
  })
}

export function useLedgerTransaction(transactionId: string) {
  return useQuery({
    queryKey: ledgerKeys.transaction(transactionId),
    queryFn: () => getLedgerTransactionById(transactionId),
    enabled: Boolean(transactionId),
  })
}

export function useLedgerTransferCandidates(transactionId: string) {
  return useQuery({
    queryKey: ledgerKeys.transferCandidates(transactionId),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.imports() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.categories() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.account(result.accountId) }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accountTransactions(result.accountId) }),
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.suggestionStatus() }),
        // the backend runs the recurring-payment detection after a commit
        qc.invalidateQueries({ queryKey: draftKeys.all }),
      ])
    },
  })
}

export function useRunLedgerRecurringDetection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: runLedgerRecurringDetection,
    meta: {
      errorMessage: (e) => (e instanceof ApiError && e.status === 409 ? i18n.t("ledger.recurring.busy") : defaultErrorMessage(e)),
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: draftKeys.all }),
  })
}

// Polls while a suggestion run (manual or the background run after an import)
// is going and refreshes the review queue once it ends.
export function useLedgerSuggestionStatus(enabled: boolean) {
  const qc = useQueryClient()
  const { data } = useQuery({
    queryKey: ledgerKeys.suggestionStatus(),
    queryFn: getLedgerCategorySuggestionStatus,
    enabled,
    refetchInterval: (query) => (query.state.data?.running ? 5000 : false),
  })
  const running = enabled && (data?.running ?? false)
  const wasRunning = useRef(running)
  useEffect(() => {
    if (wasRunning.current && !running) {
      void qc.invalidateQueries({ queryKey: ledgerKeys.review() })
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
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.suggestionStatus() }),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.categories() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.transactions() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
      ])
    },
  })
}

export function useCreateLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerAccountInput) => createLedgerAccount(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerKeys.accounts() })
    },
  })
}

export function useCreateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerCategoryInput) => createLedgerCategory(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerKeys.categories() })
    },
  })
}

export function useUpdateLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerCategoryInput }) => updateLedgerCategory(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerKeys.categories() })
    },
  })
}

export function useDeleteLedgerCategory() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerCategory(id),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerKeys.categories() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.categories() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.transaction(result.transaction.id) }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accountTransactions(result.transaction.accountId) }),
      ])
    },
  })
}

function invalidateTransactionDetails(qc: QueryClient, transaction: LedgerTransaction) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ledgerKeys.transaction(transaction.id) }),
    qc.invalidateQueries({ queryKey: ledgerKeys.transferCandidates(transaction.id) }),
    qc.invalidateQueries({ queryKey: ledgerKeys.accountTransactions(transaction.accountId) }),
    qc.invalidateQueries({ queryKey: contractKeys.all }),
    qc.invalidateQueries({ queryKey: purchaseKeys.all }),
    qc.invalidateQueries({ queryKey: vehicleKeys.all }),
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
      Promise.all([invalidateTransactionDetails(qc, transaction), qc.invalidateQueries({ queryKey: ledgerKeys.review() })]),
  })
}

// Links one target right away (the card's Save would send the whole list).
export function useAddLedgerTransactionReference() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, reference }: { id: string; reference: LedgerTransactionReference }) =>
      addLedgerTransactionReference(id, reference),
    meta: { errorMessage: (e) => i18n.t("ledger.createFromTransaction.linkFailed", { message: e.message }) },
    onSuccess: (transaction) => invalidateTransactionDetails(qc, transaction),
  })
}

function invalidateTransferPair(qc: QueryClient, result: LedgerTransferLinkResult) {
  const transactions = result.pairedTransaction ? [result.transaction, result.pairedTransaction] : [result.transaction]
  return Promise.all(
    transactions.flatMap((txn) => [
      qc.invalidateQueries({ queryKey: ledgerKeys.transaction(txn.id) }),
      qc.invalidateQueries({ queryKey: ledgerKeys.transferCandidates(txn.id) }),
      qc.invalidateQueries({ queryKey: ledgerKeys.accountTransactions(txn.accountId) }),
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
      await qc.invalidateQueries({ queryKey: ledgerKeys.emailAccounts() })
    },
  })
}

export function useUpdateLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailAccountInput }) => updateLedgerEmailAccount(id, data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerKeys.emailAccounts() })
    },
  })
}

export function useDeleteLedgerEmailAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteLedgerEmailAccount(id),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerKeys.emailAccounts() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.emailOrders() }),
      ])
    },
  })
}

function invalidateAfterEmailScan(qc: QueryClient) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ledgerKeys.emailAccounts() }),
    qc.invalidateQueries({ queryKey: ledgerKeys.emailOrders() }),
    qc.invalidateQueries({ queryKey: ledgerKeys.transactions() }),
    qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
  ])
}

// Polls while a scan of the account runs (started here, by the scheduler or
// before a page reload) and reports it once when it ends.
export function useLedgerEmailScanStatus(id: string, onFinished: (status: LedgerEmailScanStatus) => void) {
  const qc = useQueryClient()
  const { data } = useQuery({
    queryKey: ledgerKeys.emailScanStatus(id),
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
      qc.setQueryData(ledgerKeys.emailScanStatus(id), status)
    },
    // A 409 means a scan is already running: pick it up.
    onError: (_error, id) => qc.invalidateQueries({ queryKey: ledgerKeys.emailScanStatus(id) }),
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
    onSuccess: () => qc.invalidateQueries({ queryKey: draftKeys.all }),
  })
}

export function useLedgerEmailLinks(entityType: LedgerEmailLinkEntityType, entityId: string, enabled: boolean) {
  return useQuery({
    queryKey: ledgerKeys.emailLinks(entityType, entityId),
    queryFn: () => getLedgerEmailLinks(entityType, entityId),
    enabled,
  })
}

export function useLedgerEmailFolders() {
  return useMutation({
    mutationFn: (data: LedgerEmailFoldersRequest) => listLedgerEmailFolders(data),
    // The dialog shows the translated error below the folder input.
    meta: { errorToast: false },
  })
}

export function useLinkLedgerEmailOrder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: LedgerEmailOrderLinkInput }) => linkLedgerEmailOrder(id, data),
    onSuccess: async (order) => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ledgerKeys.emailOrder(order.id) }),
        qc.invalidateQueries({ queryKey: ledgerKeys.emailOrders() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.transactions() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.emailOrder(order.id) }),
        qc.invalidateQueries({ queryKey: ledgerKeys.emailOrders() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.transactions() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.accounts() }),
      ])
    },
  })
}
