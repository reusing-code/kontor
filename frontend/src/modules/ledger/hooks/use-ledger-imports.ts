import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { draftKeys } from "@/lib/query-keys"
import { ledgerKeys } from "@/modules/ledger/keys"
import { commitLedgerImport, getLedgerImports, previewLedgerImport } from "@/modules/ledger/lib/ledger-repository"
import type { LedgerCommitRequest, LedgerFileSourceType } from "@/modules/ledger/types"

export function useLedgerImports() {
  return useQuery({
    queryKey: ledgerKeys.imports(),
    queryFn: getLedgerImports,
  })
}

export function useLedgerPreviewImport() {
  return useMutation({
    mutationFn: (input: { file: File; sourceType: LedgerFileSourceType; accountId?: string }) => previewLedgerImport(input),
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
        qc.invalidateQueries({ queryKey: ledgerKeys.transactionList() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.review() }),
        qc.invalidateQueries({ queryKey: ledgerKeys.suggestionStatus() }),
        // the backend runs the recurring-payment detection after a commit
        qc.invalidateQueries({ queryKey: draftKeys.all }),
      ])
    },
  })
}
