import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ledgerKeys } from "@/modules/ledger/keys"
import { createLedgerCategory, deleteLedgerCategory, getLedgerCategories, updateLedgerCategory } from "@/modules/ledger/lib/ledger-repository"
import type { LedgerCategoryInput } from "@/modules/ledger/types"

export function useLedgerCategories() {
  return useQuery({
    queryKey: ledgerKeys.categories(),
    queryFn: getLedgerCategories,
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
