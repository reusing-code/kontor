import { useEffect, useRef } from "react"
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { draftKeys } from "@/lib/query-keys"
import { ledgerKeys } from "@/modules/ledger/keys"
import {
  createLedgerEmailAccount,
  deleteLedgerEmailAccount,
  getLedgerEmailAccounts,
  getLedgerEmailLinks,
  getLedgerEmailOrderById,
  getLedgerEmailOrderCandidates,
  getLedgerEmailOrders,
  getLedgerEmailScanStatus,
  linkLedgerEmailOrder,
  listLedgerEmailFolders,
  rejectLedgerEmailOrder,
  runLedgerEmailDrafts,
  startLedgerEmailScan,
  testLedgerEmailAccount,
  type LedgerEmailFoldersRequest,
  updateLedgerEmailAccount,
  uploadLedgerEmailMessages,
} from "@/modules/ledger/lib/ledger-repository"
import type {
  LedgerEmailAccountInput,
  LedgerEmailLinkEntityType,
  LedgerEmailOrderLinkInput,
  LedgerEmailScanStatus,
} from "@/modules/ledger/types"

export function useLedgerEmailAccounts() {
  return useQuery({
    queryKey: ledgerKeys.emailAccounts(),
    queryFn: getLedgerEmailAccounts,
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

export function useTestLedgerEmailAccount() {
  return useMutation({
    mutationFn: (id: string) => testLedgerEmailAccount(id),
  })
}

export function useLedgerEmailFolders() {
  return useMutation({
    mutationFn: (data: LedgerEmailFoldersRequest) => listLedgerEmailFolders(data),
    // The dialog shows the translated error below the folder input.
    meta: { errorToast: false },
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
