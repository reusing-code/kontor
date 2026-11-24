import { guessCostType } from "@/modules/ledger/lib/cost-type-guess"
import type { LedgerEmailOrder, LedgerEmailOrderItem, LedgerTransaction } from "@/modules/ledger/types"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { CostEntryFormData } from "@/modules/auto/types"
import type { ContractFormData } from "@/modules/contracts/types"

// Prefills of the dialogs that create an entry from a ledger transaction.

const MAX_DESCRIPTION = 200

export type OrderItem = { key: string; order: LedgerEmailOrder; item: LedgerEmailOrderItem }

export function orderItems(orders: LedgerEmailOrder[]): OrderItem[] {
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
export function singlePurchasePrefill(transaction: LedgerTransaction, orders: LedgerEmailOrder[], items: OrderItem[]): Partial<PurchaseFormData> {
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
export function itemPrefills(transaction: LedgerTransaction, items: OrderItem[]): Partial<PurchaseFormData>[] {
  return items.map((entry) =>
    itemPrefill(transaction, entry, entry.order.currency === "EUR" ? entry.item.totalPriceMinor / 100 : undefined),
  )
}

export function costPrefill(transaction: LedgerTransaction, orders: LedgerEmailOrder[]): Partial<CostEntryFormData> {
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

export function contractPrefill(transaction: LedgerTransaction): Partial<ContractFormData> {
  const name = transaction.counterpartyName?.trim()
  return compact({
    name,
    company: name,
    price: transactionEuros(transaction),
    startDate: day(transaction.bookingDate),
  })
}
