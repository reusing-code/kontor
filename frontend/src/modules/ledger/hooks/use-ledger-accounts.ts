import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ledgerKeys } from "@/modules/ledger/keys"
import {
  createLedgerAccount,
  deleteLedgerAccount,
  deleteLedgerAccountBalance,
  getLedgerAccountById,
  getLedgerAccountDeleteSummary,
  getLedgerAccounts,
  setLedgerAccountBalance,
  updateLedgerAccount,
} from "@/modules/ledger/lib/ledger-repository"
import type { LedgerAccountInput, LedgerAccountUpdateInput, LedgerManualBalanceInput } from "@/modules/ledger/types"

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

export function useCreateLedgerAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: LedgerAccountInput) => createLedgerAccount(data),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ledgerKeys.accounts() })
    },
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
