import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import i18n from "@/i18n"
import { vehicleKeys } from "@/modules/auto/keys"
import { contractKeys } from "@/modules/contracts/keys"
import { ledgerKeys } from "@/modules/ledger/keys"
import { purchaseKeys } from "@/modules/purchases/keys"
import {
  addLedgerTransactionReference,
  getAllLedgerTransactions,
  getLedgerTransactionById,
  getLedgerTransactions,
  getLedgerTransferCandidates,
  linkLedgerTransfer,
  setLedgerTransactionSplits,
  unlinkLedgerTransfer,
  updateLedgerTransactionDetails,
} from "@/modules/ledger/lib/ledger-repository"
import type {
  LedgerTransaction,
  LedgerTransactionDetailsInput,
  LedgerTransactionReference,
  LedgerTransactionSplit,
  LedgerTransferLinkInput,
  LedgerTransferLinkResult,
} from "@/modules/ledger/types"

// The shown page stays on screen while another page of the same account loads.
export function useLedgerTransactions(accountId: string, limit: number, offset: number) {
  return useQuery({
    queryKey: ledgerKeys.accountTransactionsPage(accountId, limit, offset),
    queryFn: () => getLedgerTransactions(accountId, limit, offset),
    enabled: accountId !== "",
    placeholderData: (previous, previousQuery) => (previousQuery?.queryKey[2] === accountId ? previous : undefined),
  })
}

// The shown page stays on screen while another page or selection loads.
export function useAllLedgerTransactions(accountIds: readonly string[], offset: number, enabled: boolean = true, limit: number = 100) {
  const sorted = [...accountIds].sort()
  return useQuery({
    queryKey: ledgerKeys.transactionListPage(sorted, limit, offset),
    queryFn: () => getAllLedgerTransactions(sorted, limit, offset),
    enabled,
    placeholderData: keepPreviousData,
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

function invalidateTransactionDetails(qc: QueryClient, transaction: LedgerTransaction) {
  return Promise.all([
    qc.invalidateQueries({ queryKey: ledgerKeys.transaction(transaction.id) }),
    qc.invalidateQueries({ queryKey: ledgerKeys.transferCandidates(transaction.id) }),
    qc.invalidateQueries({ queryKey: ledgerKeys.accountTransactions(transaction.accountId) }),
    qc.invalidateQueries({ queryKey: ledgerKeys.transactionList() }),
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
      qc.invalidateQueries({ queryKey: ledgerKeys.transactionList() }),
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
