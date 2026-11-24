import { useRef, useState } from "react"
import { formatMinor } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useCategories } from "@/hooks/use-categories"
import { useCreatePurchaseByCategory } from "@/modules/purchases/hooks/use-purchases"
import { useCreateCostEntryByVehicle, useVehicles } from "@/modules/auto/hooks/use-vehicles"
import type { TransactionLinker } from "@/modules/ledger/hooks/use-transaction-linker"
import { costPrefill, itemPrefills, orderItems, singlePurchasePrefill, type OrderItem } from "@/modules/ledger/lib/transaction-prefill"
import type { LedgerEmailOrder, LedgerTransaction } from "@/modules/ledger/types"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { CostEntryFormData } from "@/modules/auto/types"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { CostEntryDialog } from "@/modules/auto/components/cost-entry-dialog"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

function ItemPickerDialog({ open, items, onOpenChange, onContinue }: {
  open: boolean
  items: OrderItem[]
  onOpenChange: (open: boolean) => void
  onContinue: (items: OrderItem[]) => void
}) {
  const { t } = useTranslation()
  const [deselected, setDeselected] = useState<Set<string>>(() => new Set())
  const selected = items.filter((entry) => !deselected.has(entry.key))

  function toggle(key: string, checked: boolean) {
    setDeselected((current) => {
      const next = new Set(current)
      if (checked) next.delete(key)
      else next.add(key)
      return next
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("ledger.createFromTransaction.pickItemsTitle")}</DialogTitle>
          <DialogDescription>{t("ledger.createFromTransaction.pickItemsHint")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          {items.map((entry) => (
            <label key={entry.key} className="flex cursor-pointer items-center gap-3 rounded-md border p-3 text-sm">
              <Checkbox checked={!deselected.has(entry.key)} onCheckedChange={(checked) => toggle(entry.key, checked === true)} />
              <span className="min-w-0 flex-1 truncate">{entry.item.quantity}× {entry.item.name}</span>
              <span className="shrink-0 text-muted-foreground">
                {formatMinor(entry.item.totalPriceMinor, entry.order.currency)}
              </span>
            </label>
          ))}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button type="button" disabled={selected.length === 0} onClick={() => onContinue(selected)}>
            {t("ledger.createFromTransaction.continue", { count: selected.length })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function PurchaseFromTransaction({ transaction, orders, link }: {
  transaction: LedgerTransaction
  orders: LedgerEmailOrder[]
  link: TransactionLinker
}) {
  const { t } = useTranslation()
  const { data: categories = [] } = useCategories("purchases")
  const createPurchase = useCreatePurchaseByCategory()
  const [categoryId, setCategoryId] = useState("")
  const [pickerItems, setPickerItems] = useState<OrderItem[] | null>(null)
  // One prefilled dialog per entry, in order. The dialog closes itself after a
  // successful save and on cancel; both move on, so closing one skips it. A
  // failed save keeps it open with the entered data.
  const [queue, setQueue] = useState<Partial<PurchaseFormData>[]>([])
  const [index, setIndex] = useState(0)
  const created = useRef(0)

  function start() {
    if (!categoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    const items = orderItems(orders)
    if (items.length > 1) {
      setPickerItems(items)
      return
    }
    run([singlePurchasePrefill(transaction, orders, items)])
  }

  function run(prefills: Partial<PurchaseFormData>[]) {
    created.current = 0
    setIndex(0)
    setQueue(prefills)
  }

  function advance(current: number) {
    if (current + 1 < queue.length) {
      setIndex(current + 1)
      return
    }
    setQueue([])
    setIndex(0)
    if (created.current > 0) {
      toast.success(t("ledger.createFromTransaction.purchasesCreated", { count: created.current }))
    }
  }

  async function handleSubmit(data: PurchaseFormData) {
    const purchase = await createPurchase.mutateAsync({ categoryId, data })
    created.current += 1
    await link({ type: "purchase", targetId: purchase.id })
  }

  function handleOpenChange(open: boolean) {
    if (!open) advance(index)
  }

  return (
    <>
      <Select value={categoryId || undefined} onValueChange={setCategoryId}>
        <SelectTrigger className="w-[14rem]">
          <SelectValue placeholder={t("ledger.purchaseCategoryForCreate")} />
        </SelectTrigger>
        <SelectContent>
          {categories.map((category) => (
            <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="secondary" onClick={start} disabled={queue.length > 0}>
        {t("ledger.createFromTransaction.purchase")}
      </Button>
      {pickerItems && (
        <ItemPickerDialog
          open
          items={pickerItems}
          onOpenChange={(open) => { if (!open) setPickerItems(null) }}
          onContinue={(items) => {
            setPickerItems(null)
            run(itemPrefills(transaction, items))
          }}
        />
      )}
      <PurchaseDialog
        open={queue.length > 0}
        onOpenChange={handleOpenChange}
        initialValues={queue[index]}
        onSubmit={handleSubmit}
      />
    </>
  )
}

export function CostFromTransaction({ transaction, orders, link }: {
  transaction: LedgerTransaction
  orders: LedgerEmailOrder[]
  link: TransactionLinker
}) {
  const { t } = useTranslation()
  const { data: vehicles = [] } = useVehicles()
  const createCost = useCreateCostEntryByVehicle()
  const [vehicleId, setVehicleId] = useState("")
  // Computed when the dialog opens, so a refetch of the transaction does not
  // reset the form.
  const [initialValues, setInitialValues] = useState<Partial<CostEntryFormData> | null>(null)

  function start() {
    if (!vehicleId) {
      toast.error(t("ledger.selectVehicleFirst"))
      return
    }
    setInitialValues(costPrefill(transaction, orders))
  }

  function handleCreate(data: CostEntryFormData) {
    return createCost.mutateAsync({ vehicleId, data }, {
      onSuccess: async (entry) => {
        if (await link({ type: "cost", targetId: entry.id })) {
          toast.success(t("ledger.referenceCreatedAndLinked", { type: t("ledger.referenceTypes.cost") }))
        }
      },
    })
  }

  return (
    <>
      <Select value={vehicleId || undefined} onValueChange={setVehicleId}>
        <SelectTrigger className="w-[14rem]">
          <SelectValue placeholder={t("ledger.vehicleForCreate")} />
        </SelectTrigger>
        <SelectContent>
          {vehicles.map((vehicle) => (
            <SelectItem key={vehicle.id} value={vehicle.id}>{vehicle.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="secondary" onClick={start} disabled={createCost.isPending}>
        {t("ledger.createFromTransaction.cost")}
      </Button>
      <CostEntryDialog
        open={initialValues !== null}
        onOpenChange={(open) => { if (!open) setInitialValues(null) }}
        initialValues={initialValues ?? undefined}
        onSubmit={handleCreate}
      />
    </>
  )
}
