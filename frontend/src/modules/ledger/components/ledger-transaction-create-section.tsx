import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useCategories } from "@/hooks/use-categories"
import { useModules } from "@/hooks/use-modules"
import { useCreateContractByCategory } from "@/modules/contracts/hooks/use-contracts"
import { useCreateVehicle } from "@/modules/auto/hooks/use-vehicles"
import type { ContractFormData } from "@/modules/contracts/types"
import type { LedgerEmailOrder, LedgerTransaction } from "@/modules/ledger/types"
import type { VehicleFormData } from "@/modules/auto/types"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { VehicleDialog } from "@/modules/auto/components/vehicle-dialog"
import { CostFromTransaction, PurchaseFromTransaction } from "@/modules/ledger/components/ledger-transaction-create-actions"
import type { TransactionLinker } from "@/modules/ledger/hooks/use-transaction-linker"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

function ContractCreateAndLink({ transaction, link }: { transaction: LedgerTransaction; link: TransactionLinker }) {
  const { t } = useTranslation()
  const { data: contractCategories = [] } = useCategories("contracts")
  const createContract = useCreateContractByCategory()
  const [initialValues, setInitialValues] = useState<Partial<ContractFormData> | null>(null)
  const [categoryId, setCategoryId] = useState("")

  function openDialog() {
    if (!categoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    const name = transaction.counterpartyName?.trim()
    setInitialValues({
      ...(name ? { name, company: name } : {}),
      ...(transaction.currency === "EUR" ? { price: Math.abs(transaction.amountMinor) / 100 } : {}),
      startDate: transaction.bookingDate.slice(0, 10),
    })
  }

  function handleCreate(data: ContractFormData) {
    return createContract.mutateAsync({ categoryId, data }, {
      onSuccess: async (contract) => {
        if (await link({ type: "contract", targetId: contract.id })) {
          toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.contracts") }))
        }
      },
    })
  }

  return (
    <>
      <Select value={categoryId || undefined} onValueChange={setCategoryId}>
        <SelectTrigger className="w-[14rem]">
          <SelectValue placeholder={t("ledger.contractCategoryForCreate")} />
        </SelectTrigger>
        <SelectContent>
          {contractCategories.map((category) => (
            <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="secondary" onClick={openDialog}>{t("ledger.createContractAndLink")}</Button>
      <ContractDialog
        open={initialValues !== null}
        onOpenChange={(open) => { if (!open) setInitialValues(null) }}
        initialValues={initialValues ?? undefined}
        onSubmit={handleCreate}
      />
    </>
  )
}

function VehicleCreateAndLink({ link }: { link: TransactionLinker }) {
  const { t } = useTranslation()
  const createVehicle = useCreateVehicle()
  const [dialogOpen, setDialogOpen] = useState(false)

  function handleCreate(data: VehicleFormData) {
    return createVehicle.mutateAsync(data, {
      onSuccess: async (vehicle) => {
        if (await link({ type: "vehicle", targetId: vehicle.id })) {
          toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.auto") }))
        }
      },
    })
  }

  return (
    <>
      <Button type="button" variant="secondary" onClick={() => setDialogOpen(true)}>{t("ledger.createVehicleAndLink")}</Button>
      <VehicleDialog open={dialogOpen} onOpenChange={setDialogOpen} onSubmit={handleCreate} />
    </>
  )
}

export function LedgerTransactionCreateSection({ transaction, orders, link }: {
  transaction: LedgerTransaction
  orders: LedgerEmailOrder[]
  link: TransactionLinker
}) {
  const { t } = useTranslation()
  const { isEnabled } = useModules()

  return (
    <div className="space-y-2 rounded-md border border-dashed p-3">
      <div className="text-sm font-medium">{t("ledger.createFromTransaction.title")}</div>
      <p className="text-xs text-muted-foreground">{t("ledger.createFromTransaction.hint")}</p>
      {isEnabled("purchases") && (
        <div className="flex flex-wrap gap-2">
          <PurchaseFromTransaction transaction={transaction} orders={orders} link={link} />
        </div>
      )}
      {isEnabled("auto") && (
        <div className="flex flex-wrap gap-2">
          <CostFromTransaction transaction={transaction} orders={orders} link={link} />
        </div>
      )}
      {(isEnabled("contracts") || isEnabled("auto")) && (
        <div className="flex flex-wrap gap-2">
          {isEnabled("contracts") && <ContractCreateAndLink transaction={transaction} link={link} />}
          {isEnabled("auto") && <VehicleCreateAndLink link={link} />}
        </div>
      )}
    </div>
  )
}
