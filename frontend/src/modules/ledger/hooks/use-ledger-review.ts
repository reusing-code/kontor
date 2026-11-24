import { useEffect, useRef } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ledgerKeys } from "@/modules/ledger/keys"
import {
  getLedgerCategorySuggestionStatus,
  getLedgerReviewCount,
  getLedgerReviewQueueAll,
  reviewLedgerTransaction,
  reviewLedgerTransactionsBatch,
  runLedgerCategorySuggestions,
} from "@/modules/ledger/lib/ledger-repository"
import type { LedgerBatchReviewItem, LedgerReviewInput } from "@/modules/ledger/types"

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
        qc.invalidateQueries({ queryKey: ledgerKeys.transactionList() }),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.transactionList() }),
      ])
    },
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
