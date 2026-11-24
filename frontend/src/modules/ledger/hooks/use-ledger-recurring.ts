import { useMutation, useQueryClient } from "@tanstack/react-query"
import i18n from "@/i18n"
import { ApiError } from "@/lib/api"
import { defaultErrorMessage } from "@/lib/query-client"
import { draftKeys } from "@/lib/query-keys"
import { runLedgerRecurringDetection } from "@/modules/ledger/lib/ledger-repository"

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
