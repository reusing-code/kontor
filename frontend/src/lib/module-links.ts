import type { LedgerTransactionReference } from "@/types/ledger"

export function moduleReferenceToPath(reference: LedgerTransactionReference): string {
  switch (reference.type) {
    case "purchase":
      return `/purchases/${reference.targetId}`
    case "contract":
      return `/contracts/${reference.targetId}`
    case "vehicle":
      return `/auto/vehicles/${reference.targetId}`
  }
}

export function transactionPath(transactionId: string): string {
  return `/ledger/transactions/${transactionId}`
}
