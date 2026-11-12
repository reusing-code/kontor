import { useMemo } from "react"
import { useCreatePurchaseByCategory } from "@/modules/purchases/hooks/use-purchases"
import { useCreateContractByCategory } from "@/modules/contracts/hooks/use-contracts"
import { useCreateCostEntryByVehicle, useCreateVehicle } from "@/modules/auto/hooks/use-vehicles"
import { useCreateCategory } from "@/hooks/use-categories"
import { useCreateLedgerAccount, useCreateLedgerCategory, useLedgerCategories } from "@/modules/ledger/hooks/use-ledger"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { CostEntryDialog } from "@/modules/auto/components/cost-entry-dialog"
import { VehicleDialog } from "@/modules/auto/components/vehicle-dialog"
import { CategoryDialog } from "@/components/category-dialog"
import { LedgerCategoryDialog } from "@/modules/ledger/components/ledger-category-dialog"
import { LedgerAccountDialog } from "@/modules/ledger/components/ledger-account-dialog"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { ContractFormData } from "@/modules/contracts/types"
import type { CostEntryFormData, VehicleFormData } from "@/modules/auto/types"
import type { CategoryFormData } from "@/types/category"
import type { LedgerAccountInput, LedgerCategoryInput } from "@/modules/ledger/types"
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
  const createVehicle = useCreateVehicle()
  const createContractCategory = useCreateCategory("contracts")
  const createPurchaseCategory = useCreateCategory("purchases")
  const createLedgerCategory = useCreateLedgerCategory()
  const createLedgerAccount = useCreateLedgerAccount()
  const { data: ledgerCategories = [] } = useLedgerCategories()

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
  const vehicleInitial = useMemo(
    () => (entry?.type === "vehicle" ? initialValues<VehicleFormData>("vehicle", entry.input) : undefined),
    [entry],
  )
  const contractCategoryInitial = useMemo(
    () => (entry?.type === "contract_category" ? initialValues<CategoryFormData>("contract_category", entry.input) : undefined),
    [entry],
  )
  const purchaseCategoryInitial = useMemo(
    () => (entry?.type === "purchase_category" ? initialValues<CategoryFormData>("purchase_category", entry.input) : undefined),
    [entry],
  )
  const ledgerCategoryInitial = useMemo(
    () => (entry?.type === "ledger_category" ? initialValues<LedgerCategoryInput>("ledger_category", entry.input) : undefined),
    [entry],
  )
  const ledgerAccountInitial = useMemo(
    () => (entry?.type === "ledger_account" ? initialValues<LedgerAccountInput>("ledger_account", entry.input) : undefined),
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
      <VehicleDialog
        open={entry?.type === "vehicle"}
        onOpenChange={handleOpenChange}
        initialValues={vehicleInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createVehicle.mutateAsync(data))
        }}
      />
      <CategoryDialog
        open={entry?.type === "contract_category"}
        onOpenChange={handleOpenChange}
        initialValues={contractCategoryInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createContractCategory.mutateAsync(data))
        }}
      />
      <CategoryDialog
        open={entry?.type === "purchase_category"}
        onOpenChange={handleOpenChange}
        initialValues={purchaseCategoryInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createPurchaseCategory.mutateAsync(data))
        }}
      />
      <LedgerCategoryDialog
        open={entry?.type === "ledger_category"}
        onOpenChange={handleOpenChange}
        categories={ledgerCategories}
        initialParentId={entry?.type === "ledger_category" && entry.parentId ? entry.parentId : undefined}
        initialValues={ledgerCategoryInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createLedgerCategory.mutateAsync(data))
        }}
      />
      <LedgerAccountDialog
        open={entry?.type === "ledger_account"}
        onOpenChange={handleOpenChange}
        initialValues={ledgerAccountInitial}
        onSubmit={(data) => {
          if (entry) onCreate(entry, () => createLedgerAccount.mutateAsync(data))
        }}
      />
    </>
  )
}
