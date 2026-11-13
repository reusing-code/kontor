import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useCategories } from "@/hooks/use-categories"
import { useCreatePurchaseByCategory } from "@/modules/purchases/hooks/use-purchases"
import { useCreateCostEntryByVehicle, useVehicles } from "@/modules/auto/hooks/use-vehicles"
import type { TransactionLinker } from "@/modules/ledger/hooks/use-transaction-linker"
import { guessCostType } from "@/modules/ledger/lib/cost-type-guess"
import { formatAmountMinor } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerEmailOrder, LedgerEmailOrderItem, LedgerTransaction } from "@/modules/ledger/types"
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

const MAX_DESCRIPTION = 200

type OrderItem = { key: string; order: LedgerEmailOrder; item: LedgerEmailOrderItem }

function orderItems(orders: LedgerEmailOrder[]): OrderItem[] {
  return orders.flatMap((order) => (order.items ?? []).map((item, index) => ({ key: `${order.id}:${index}`, order, item })))
}

function compact<T extends object>(values: T): Partial<T> {
  return Object.fromEntries(Object.entries(values).filter(([, value]) => value !== undefined && value !== "")) as Partial<T>
}

function day(value?: string): string | undefined {
  return value ? value.slice(0, 10) : undefined
}

function truncate(value: string | undefined, max: number): string | undefined {
  if (!value) return undefined
  const trimmed = value.trim()
  return trimmed.length > max ? `${trimmed.slice(0, max - 1).trimEnd()}…` : trimmed
}

function transactionEuros(transaction: LedgerTransaction): number | undefined {
  return transaction.currency === "EUR" ? Math.abs(transaction.amountMinor) / 100 : undefined
}

function itemPrefill(transaction: LedgerTransaction, { order, item }: OrderItem, price: number | undefined): Partial<PurchaseFormData> {
  return compact({
    itemName: item.name,
    dealer: order.merchant || transaction.counterpartyName,
    price,
    purchaseDate: day(order.orderDate) || day(transaction.bookingDate),
  })
}

// With one item (or no order) the purchase gets the transaction amount.
function singlePurchasePrefill(transaction: LedgerTransaction, orders: LedgerEmailOrder[], items: OrderItem[]): Partial<PurchaseFormData> {
  if (items.length === 1) {
    return itemPrefill(transaction, items[0], transactionEuros(transaction))
  }
  const order = orders[0]
  return compact({
    dealer: order?.merchant || transaction.counterpartyName,
    price: transactionEuros(transaction),
    purchaseDate: day(order?.orderDate) || day(transaction.bookingDate),
  })
}

// Items picked from a multi-item order keep their own price, known only in EUR.
function itemPrefills(transaction: LedgerTransaction, items: OrderItem[]): Partial<PurchaseFormData>[] {
  return items.map((entry) =>
    itemPrefill(transaction, entry, entry.order.currency === "EUR" ? entry.item.totalPriceMinor / 100 : undefined),
  )
}

function costPrefill(transaction: LedgerTransaction, orders: LedgerEmailOrder[]): Partial<CostEntryFormData> {
  const merchant = orders.find((order) => order.merchant)?.merchant
  const itemNames = orderItems(orders).map(({ item }) => item.name).join(", ")
  return compact({
    type: guessCostType(transaction.counterpartyName, transaction.purpose, merchant),
    vendor: merchant || transaction.counterpartyName,
    amount: transactionEuros(transaction),
    date: day(transaction.bookingDate),
    description: truncate(itemNames || transaction.purpose, MAX_DESCRIPTION),
  })
}

function ItemPickerDialog({ open, items, onOpenChange, onContinue }: {
  open: boolean
  items: OrderItem[]
  onOpenChange: (open: boolean) => void
  onContinue: (items: OrderItem[]) => void
}) {
  const { t, i18n } = useTranslation()
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
                {formatAmountMinor(entry.item.totalPriceMinor, entry.order.currency, i18n.language)}
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
  // One prefilled dialog per entry, in order; closing a dialog skips its entry.
  const [queue, setQueue] = useState<Partial<PurchaseFormData>[]>([])
  const [index, setIndex] = useState(0)
  const [saving, setSaving] = useState(false)
  // The dialog closes itself right after onSubmit, which must not skip.
  const submitted = useRef(false)
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

  async function save(current: number, data: PurchaseFormData) {
    setSaving(true)
    try {
      let purchaseId: string
      try {
        purchaseId = (await createPurchase.mutateAsync({ categoryId, data })).id
      } catch (error) {
        toast.error(t("ledger.createFromTransaction.createFailed", { message: (error as Error).message }))
        // reopen the same entry with what the user entered
        setQueue((entries) => entries.map((entry, i) => (i === current ? data : entry)))
        return
      }
      created.current += 1
      await link({ type: "purchase", targetId: purchaseId })
      advance(current)
    } finally {
      setSaving(false)
    }
  }

  function handleSubmit(data: PurchaseFormData) {
    submitted.current = true
    void save(index, data)
  }

  function handleOpenChange(open: boolean) {
    if (open) return
    if (submitted.current) {
      submitted.current = false
      return
    }
    advance(index)
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
      <Button type="button" variant="secondary" onClick={start} disabled={saving || queue.length > 0}>
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
        open={queue.length > 0 && !saving}
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
    createCost.mutate({ vehicleId, data }, {
      onSuccess: async (entry) => {
        if (await link({ type: "cost", targetId: entry.id })) {
          toast.success(t("ledger.referenceCreatedAndLinked", { type: t("ledger.referenceTypes.cost") }))
        }
      },
      onError: (error) => toast.error(t("ledger.createFromTransaction.createFailed", { message: error.message })),
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
