import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  commitLedgerImport,
  getLedgerAccountById,
  getLedgerAccounts,
  getLedgerImports,
  getLedgerTransactions,
  previewLedgerImport,
} from "@/lib/ledger-repository"
import type { LedgerCommitRequest, LedgerSourceType } from "@/types/ledger"

export const ledgerAccountsKey = ["ledger", "accounts"] as const
export const ledgerImportsKey = ["ledger", "imports"] as const
const ledgerAccountKey = (accountId: string) => ["ledger", "accounts", accountId] as const
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
      qc.invalidateQueries({ queryKey: ledgerAccountKey(result.accountId) })
      qc.invalidateQueries({ queryKey: ["ledger", "accounts", result.accountId, "transactions"] })
    },
  })
}
