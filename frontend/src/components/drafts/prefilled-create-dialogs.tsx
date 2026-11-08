import { useMemo } from "react"
import { useCreatePurchaseByCategory } from "@/modules/purchases/hooks/use-purchases"
import { useCreateContractByCategory } from "@/modules/contracts/hooks/use-contracts"
import { useCreateCostEntryByVehicle } from "@/modules/auto/hooks/use-vehicles"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { CostEntryDialog } from "@/modules/auto/components/cost-entry-dialog"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { ContractFormData } from "@/modules/contracts/types"
import type { CostEntryFormData } from "@/modules/auto/types"
import { initialValues, type PrefillEntry } from "@/components/drafts/draft-input"

interface PrefilledCreateDialogsProps {
  entry: PrefillEntry | null
  onClose: () => void
  // create runs the module's create mutation with the submitted form data.
  onCreate: (entry: PrefillEntry, create: () => Promise<{ id: string }>) => void
}

// PrefilledCreateDialogs opens the create dialog of entry's module with the
// entry's fields filled in.
export function PrefilledCreateDialogs({ entry, onClose, onCreate }: PrefilledCreateDialogsProps) {
  const createPurchase = useCreatePurchaseByCategory()
  const createContract = useCreateContractByCategory()
  const createCost = useCreateCostEntryByVehicle()

  const purchaseInitial = useMemo(
    () => (entry?.type === "purchase" ? initialValues<PurchaseFormData>("purchase", entry.input) : undefined),
    [entry],
  )
  const contractInitial = useMemo(
    () => (entry?.type === "contract" ? initialValues<ContractFormData>("contract", entry.input) : undefined),
    [entry],
  )
  const costInitial = useMemo(
    () => (entry?.type === "cost" ? initialValues<CostEntryFormData>("cost", entry.input) : undefined),
    [entry],
  )

  function handleOpenChange(open: boolean) {
    if (!open) onClose()
  }

  return (
    <>
      <PurchaseDialog
        open={entry?.type === "purchase"}
        onOpenChange={handleOpenChange}
        initialValues={purchaseInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createPurchase.mutateAsync({ categoryId: entry.parentId, data }))
        }}
      />
      <ContractDialog
        open={entry?.type === "contract"}
        onOpenChange={handleOpenChange}
        initialValues={contractInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createContract.mutateAsync({ categoryId: entry.parentId, data }))
        }}
      />
      <CostEntryDialog
        open={entry?.type === "cost"}
        onOpenChange={handleOpenChange}
        initialValues={costInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createCost.mutateAsync({ vehicleId: entry.parentId, data }))
        }}
      />
    </>
  )
}
