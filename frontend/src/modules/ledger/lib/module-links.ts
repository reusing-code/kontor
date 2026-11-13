import type { LedgerTransactionReference } from "@/modules/ledger/types"
import type { ModuleId } from "@/types/modules"

export function referenceModuleId(reference: LedgerTransactionReference): ModuleId {
  switch (reference.type) {
    case "purchase":
      return "purchases"
    case "contract":
      return "contracts"
    case "vehicle":
    case "cost":
      return "auto"
  }
}

export function vehiclePath(vehicleId: string): string {
  return `/auto/vehicles/${vehicleId}`
}

// A cost reference only carries the cost ID; its page is the vehicle's, so
// callers resolve the cost first and use vehiclePath.
export function moduleReferenceToPath(reference: LedgerTransactionReference): string | null {
  switch (reference.type) {
    case "purchase":
      return `/purchases/${reference.targetId}`
    case "contract":
      return `/contracts/${reference.targetId}`
    case "vehicle":
      return vehiclePath(reference.targetId)
    case "cost":
      return null
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
