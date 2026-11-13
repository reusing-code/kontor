import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import type { TFunction } from "i18next"
import { ApiError } from "@/lib/api"
import { useAuthorizeBank } from "@/modules/ledger/hooks/use-bank-connections"
import type { BankAuthorizeInput } from "@/modules/ledger/types"

// Fixed endpoint codes get a translated message; anything else shows the
// endpoint's own (fixed, English) text.
export function bankErrorMessage(t: TFunction, error: Error): string {
  if (error instanceof ApiError && error.code) {
    return t(`ledger.bankImport.errors.${error.code}`, { defaultValue: error.message })
  }
  return error.message
}

// Starts a consent and sends the browser to the bank.
export function useStartBankAuthorization() {
  const { t } = useTranslation()
  const authorize = useAuthorizeBank()

  function start(input: BankAuthorizeInput) {
    authorize.mutate(input, {
      onSuccess: ({ url }) => window.location.assign(url),
      onError: (error) => toast.error(bankErrorMessage(t, error)),
    })
  }

  return { start, isPending: authorize.isPending }
}
