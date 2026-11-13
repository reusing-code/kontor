import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useAddLedgerTransactionReference } from "@/modules/ledger/hooks/use-ledger"
import type { LedgerTransactionReference } from "@/modules/ledger/types"

// Links a newly created entity to the transaction. Resolves to false when the
// link failed; the reference is still kept locally so Save can retry it.
export type TransactionLinker = (reference: LedgerTransactionReference) => Promise<boolean>

export function useTransactionLinker(transactionId: string, onLinked: (reference: LedgerTransactionReference) => void): TransactionLinker {
  const { t } = useTranslation()
  const addReference = useAddLedgerTransactionReference()
  return async (reference) => {
    onLinked(reference)
    try {
      await addReference.mutateAsync({ id: transactionId, reference })
      return true
    } catch (error) {
      toast.error(t("ledger.createFromTransaction.linkFailed", { message: (error as Error).message }))
      return false
    }
  }
}
