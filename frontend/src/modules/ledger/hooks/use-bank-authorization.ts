import { useAuthorizeBank } from "@/modules/ledger/hooks/use-bank-connections"
import type { BankAuthorizeInput } from "@/modules/ledger/types"

// Starts a consent and sends the browser to the bank.
export function useStartBankAuthorization() {
  const authorize = useAuthorizeBank()

  function start(input: BankAuthorizeInput) {
    authorize.mutate(input, {
      onSuccess: ({ url }) => window.location.assign(url),
    })
  }

  return { start, isPending: authorize.isPending }
}
