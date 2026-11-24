import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useCapabilities } from "@/hooks/use-capabilities"
import i18n from "@/i18n"
import type { KontorMutationMeta } from "@/lib/query-client"
import { ledgerKeys } from "@/modules/ledger/keys"
import { bankErrorMessage } from "@/modules/ledger/lib/bank-errors"
import {
  authorizeBank,
  disconnectBank,
  getBankAspsps,
  getBankConnections,
  mapBankAccount,
  syncBankConnection,
  unmapBankAccount,
} from "@/modules/ledger/lib/bank-repository"

const bankErrorMeta: KontorMutationMeta = { errorMessage: (e) => bankErrorMessage(i18n.t, e) }

// Without the capability the query never runs, so a server without Enable
// Banking is not asked.
export function useBankConnections() {
  const { bankImport } = useCapabilities()
  return useQuery({
    queryKey: ledgerKeys.bankConnections(),
    queryFn: getBankConnections,
    enabled: bankImport,
    staleTime: 60_000,
  })
}

export function useBankAspsps(country: string, enabled: boolean) {
  return useQuery({
    queryKey: ledgerKeys.bankAspsps(country),
    queryFn: () => getBankAspsps(country),
    enabled,
    staleTime: 10 * 60_000,
    retry: false,
  })
}

export function useAuthorizeBank() {
  return useMutation({ mutationFn: authorizeBank, meta: bankErrorMeta })
}

export function useDisconnectBank() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: disconnectBank,
    meta: bankErrorMeta,
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.bankConnections() }),
  })
}

export function useMapBankAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, uid, ledgerAccountId }: { id: string; uid: string; ledgerAccountId: string }) =>
      mapBankAccount(id, uid, ledgerAccountId),
    meta: bankErrorMeta,
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.bankConnections() }),
  })
}

export function useUnmapBankAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, uid }: { id: string; uid: string }) => unmapBankAccount(id, uid),
    meta: bankErrorMeta,
    onSuccess: () => qc.invalidateQueries({ queryKey: ledgerKeys.bankConnections() }),
  })
}

// A sync stores transactions, so every ledger query (accounts, imports, the
// review queue) is refreshed.
export function useSyncBankConnection() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: syncBankConnection,
    meta: bankErrorMeta,
    onSettled: () => qc.invalidateQueries({ queryKey: ledgerKeys.all }),
  })
}
