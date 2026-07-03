import type { LedgerTransactionReference } from "@/modules/ledger/types"
import type { ModuleId } from "@/types/modules"

export function referenceModuleId(reference: LedgerTransactionReference): ModuleId {
  switch (reference.type) {
    case "purchase":
      return "purchases"
    case "contract":
      return "contracts"
    case "vehicle":
      return "auto"
  }
}

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

export function moduleReferenceToEnabledPath(
  reference: LedgerTransactionReference,
  enabledModules: ModuleId[],
): string | null {
  return enabledModules.includes(referenceModuleId(reference)) ? moduleReferenceToPath(reference) : null
}

export function transactionPath(transactionId: string): string {
  return `/ledger/transactions/${transactionId}`
}
