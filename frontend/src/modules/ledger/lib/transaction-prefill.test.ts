import { describe, expect, it } from "vitest"
import {
  contractPrefill,
  costPrefill,
  itemPrefills,
  orderItems,
  singlePurchasePrefill,
} from "@/modules/ledger/lib/transaction-prefill"
import type { LedgerEmailOrder, LedgerEmailOrderItem, LedgerTransaction } from "@/modules/ledger/types"

const at = "2026-11-01T10:00:00Z"

function txn(overrides: Partial<LedgerTransaction> = {}): LedgerTransaction {
  return {
    id: "txn-1",
    accountId: "acc",
    bookingDate: "2026-11-02",
    amountMinor: -5998,
    currency: "EUR",
    counterpartyName: "PayPal",
    purpose: "Order 123",
    reviewStatus: "confirmed",
    categorizationSource: "manual",
    sourceType: "dkb.csv",
    importBatchId: "batch",
    fingerprint: "fp",
    createdAt: at,
    updatedAt: at,
    ...overrides,
  }
}

function item(name: string, totalPriceMinor: number): LedgerEmailOrderItem {
  return { name, quantity: 1, unitPriceMinor: totalPriceMinor, totalPriceMinor }
}

function order(id: string, items: LedgerEmailOrderItem[], overrides: Partial<LedgerEmailOrder> = {}): LedgerEmailOrder {
  return {
    id,
    emailAccountId: "mail",
    importerId: "shop",
    orderDate: "2026-10-30T18:45:00Z",
    totalMinor: items.reduce((sum, i) => sum + i.totalPriceMinor, 0),
    currency: "EUR",
    merchant: "Shop",
    items,
    matchStatus: "matched",
    createdAt: at,
    updatedAt: at,
    ...overrides,
  }
}

describe("orderItems", () => {
  it("lists the items of all orders with a key per order and position", () => {
    const a = order("a", [item("Lamp", 2999), item("Shelf", 2999)])
    const b = order("b", [item("Cable", 500)])
    const none = order("c", [], { items: undefined })
    expect(orderItems([a, b, none]).map(({ key, item: i }) => [key, i.name])).toEqual([
      ["a:0", "Lamp"],
      ["a:1", "Shelf"],
      ["b:0", "Cable"],
    ])
  })
})

describe("singlePurchasePrefill", () => {
  it("takes the counterparty, the transaction amount without sign and the booking date without an order", () => {
    expect(singlePurchasePrefill(txn(), [], [])).toEqual({ dealer: "PayPal", price: 59.98, purchaseDate: "2026-11-02" })
  })

  it("uses the amount of an incoming transaction as well", () => {
    expect(singlePurchasePrefill(txn({ amountMinor: 1250 }), [], [])).toMatchObject({ price: 12.5 })
  })

  it("leaves the price empty for a transaction in another currency", () => {
    expect(singlePurchasePrefill(txn({ currency: "USD" }), [], [])).toEqual({ dealer: "PayPal", purchaseDate: "2026-11-02" })
  })

  it("names the purchase after the only item and gives it the transaction amount", () => {
    const orders = [order("a", [item("Lamp", 4999)])]
    expect(singlePurchasePrefill(txn(), orders, orderItems(orders))).toEqual({
      itemName: "Lamp", dealer: "Shop", price: 59.98, purchaseDate: "2026-10-30",
    })
  })

  it("takes the transaction amount for the only item of an order in another currency", () => {
    const orders = [order("a", [item("Lamp", 4999)], { currency: "USD" })]
    expect(singlePurchasePrefill(txn(), orders, orderItems(orders))).toMatchObject({ itemName: "Lamp", price: 59.98 })
  })

  it("takes merchant and order date of an order without items", () => {
    const orders = [order("a", [])]
    expect(singlePurchasePrefill(txn(), orders, [])).toEqual({ dealer: "Shop", price: 59.98, purchaseDate: "2026-10-30" })
  })

  it("falls back to the counterparty and booking date when the order has neither", () => {
    const orders = [order("a", [item("Lamp", 4999)], { merchant: undefined, orderDate: "" })]
    expect(singlePurchasePrefill(txn(), orders, orderItems(orders))).toEqual({
      itemName: "Lamp", dealer: "PayPal", price: 59.98, purchaseDate: "2026-11-02",
    })
  })
})

describe("itemPrefills", () => {
  it("gives each picked item its own price in EUR orders and none in other currencies", () => {
    const eur = order("a", [item("Lamp", 2999), item("Shelf", 4500)])
    const usd = order("b", [item("Cable", 999)], { currency: "USD", merchant: "US Shop", orderDate: "2026-10-28" })
    expect(itemPrefills(txn(), orderItems([eur, usd]))).toEqual([
      { itemName: "Lamp", dealer: "Shop", price: 29.99, purchaseDate: "2026-10-30" },
      { itemName: "Shelf", dealer: "Shop", price: 45, purchaseDate: "2026-10-30" },
      { itemName: "Cable", dealer: "US Shop", purchaseDate: "2026-10-28" },
    ])
  })

  it("is empty when nothing is picked", () => {
    expect(itemPrefills(txn(), [])).toEqual([])
  })
})

describe("costPrefill", () => {
  it("takes type, vendor, amount, date and purpose from the transaction without an order", () => {
    expect(costPrefill(txn({ counterpartyName: "ARAL Station 12", purpose: "Tankstelle Berlin" }), [])).toEqual({
      type: "fuel", vendor: "ARAL Station 12", amount: 59.98, date: "2026-11-02", description: "Tankstelle Berlin",
    })
  })

  it("describes the cost by the order items and names the merchant as vendor", () => {
    const orders = [
      order("a", [], { merchant: undefined }),
      order("b", [item("Winter tyres", 40000), item("Valves", 1000)], { merchant: "Reifen Direkt" }),
    ]
    expect(costPrefill(txn(), orders)).toEqual({
      type: "tires", vendor: "Reifen Direkt", amount: 59.98, date: "2026-11-02", description: "Winter tyres, Valves",
    })
  })

  it("uses the booking date, not the order date, and no amount in another currency", () => {
    const orders = [order("a", [item("Oil", 3000)])]
    expect(costPrefill(txn({ currency: "CHF", bookingDate: "2026-11-03T00:00:00Z" }), orders)).toEqual({
      type: "misc", vendor: "Shop", date: "2026-11-03", description: "Oil",
    })
  })

  it("shortens a long description to 200 characters", () => {
    const description = costPrefill(txn({ purpose: `  ${"x".repeat(150)} ${"y".repeat(100)}  ` }), []).description ?? ""
    expect(description).toHaveLength(200)
    expect(description.endsWith("…")).toBe(true)
    expect(costPrefill(txn({ purpose: `  ${"z".repeat(200)}  ` }), []).description).toBe("z".repeat(200))
  })

  it("leaves out what the transaction does not have", () => {
    expect(costPrefill(txn({ counterpartyName: undefined, purpose: undefined }), [])).toEqual({
      type: "misc", amount: 59.98, date: "2026-11-02",
    })
  })
})

describe("contractPrefill", () => {
  it("takes the counterparty as name and company, the amount without sign and the booking day as start", () => {
    expect(contractPrefill(txn({ counterpartyName: "  FitCo GmbH ", bookingDate: "2026-11-02T08:00:00Z", amountMinor: -2990 }))).toEqual({
      name: "FitCo GmbH", company: "FitCo GmbH", price: 29.9, startDate: "2026-11-02",
    })
  })

  it("leaves the name empty for a blank counterparty and the price for another currency", () => {
    expect(contractPrefill(txn({ counterpartyName: "  ", currency: "GBP" }))).toEqual({ startDate: "2026-11-02" })
  })
})
