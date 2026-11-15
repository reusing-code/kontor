import type { ApiCall } from "@/test/api-mock"
import type { Draft, DraftEntityType, DraftTargetType } from "@/types/drafts"
import type { ModuleId } from "@/types/modules"

export const ids = {
  purchaseCategory: "11111111-1111-4111-8111-111111111111",
  contractCategory: "22222222-2222-4222-8222-222222222222",
  vehicle: "33333333-3333-4333-8333-333333333333",
  ledgerCategory: "44444444-4444-4444-8444-444444444444",
  created: "99999999-9999-4999-8999-999999999999",
}

const stamp = "2026-05-01T08:00:00Z"

export function draft(targetType: DraftTargetType, input: Record<string, unknown>, extra: Partial<Draft> = {}): Draft {
  return {
    id: `draft-${targetType}`,
    status: "pending",
    targetType,
    input,
    source: { kind: "mcp", ref: `ref-${targetType}`, title: `Draft ${targetType}` },
    createdAt: stamp,
    updatedAt: stamp,
    ...extra,
  }
}

export interface AcceptCase {
  type: DraftEntityType
  parentId?: string
  // The draft input; "unknownField" and the empty values must not reach the dialog.
  input: Record<string, unknown>
  initialValues: Record<string, unknown>
  dialogTitle: string
  // Values the opened form shows.
  shown: string[]
  createPath: string
  // Fields the create request must carry.
  created: Record<string, unknown>
  entityPath: string
}

const noise = { unknownField: "dropped", comments: "" }

export const acceptCases: AcceptCase[] = [
  {
    type: "purchase",
    parentId: ids.purchaseCategory,
    input: { ...noise, itemName: "Standing desk", dealer: "Deskhaus", price: 499.9, purchaseDate: "2026-04-02", brand: null },
    initialValues: { itemName: "Standing desk", dealer: "Deskhaus", price: 499.9, purchaseDate: "2026-04-02" },
    dialogTitle: "Add Purchase",
    shown: ["Standing desk", "Deskhaus", "499.9", "2026-04-02"],
    createPath: `/categories/${ids.purchaseCategory}/purchases`,
    created: { itemName: "Standing desk", dealer: "Deskhaus", price: 499.9, purchaseDate: "2026-04-02" },
    entityPath: `/purchases/${ids.created}`,
  },
  {
    type: "contract",
    parentId: ids.contractCategory,
    input: { ...noise, name: "Fiber 500", company: "NetCo", price: 39.99, billingInterval: "yearly", startDate: "2026-01-01" },
    initialValues: { name: "Fiber 500", company: "NetCo", price: 39.99, billingInterval: "yearly", startDate: "2026-01-01" },
    dialogTitle: "Add Contract",
    shown: ["Fiber 500", "NetCo", "39.99", "2026-01-01"],
    createPath: `/categories/${ids.contractCategory}/contracts`,
    created: { name: "Fiber 500", company: "NetCo", price: 39.99, billingInterval: "yearly", startDate: "2026-01-01" },
    entityPath: `/contracts/${ids.created}`,
  },
  {
    type: "cost",
    parentId: ids.vehicle,
    input: { ...noise, type: "fuel", vendor: "Shell", amount: 61.5, date: "2026-03-03", mileage: 42000 },
    initialValues: { type: "fuel", vendor: "Shell", amount: 61.5, date: "2026-03-03", mileage: 42000 },
    dialogTitle: "Add Cost Entry",
    shown: ["Shell", "61.5", "2026-03-03", "42000"],
    createPath: `/vehicles/${ids.vehicle}/costs`,
    created: { type: "fuel", vendor: "Shell", amount: 61.5, date: "2026-03-03", mileage: 42000 },
    entityPath: `/auto/vehicles/${ids.vehicle}`,
  },
  {
    type: "vehicle",
    input: { ...noise, name: "Family car", make: "VW", model: "Golf VII", licensePlate: "B-XY 123", year: 2019 },
    initialValues: { name: "Family car", make: "VW", model: "Golf VII", licensePlate: "B-XY 123", year: 2019 },
    dialogTitle: "Add Vehicle",
    shown: ["Family car", "VW", "Golf VII", "B-XY 123", "2019"],
    createPath: "/vehicles",
    created: { name: "Family car", make: "VW", model: "Golf VII", licensePlate: "B-XY 123", year: 2019 },
    entityPath: `/auto/vehicles/${ids.created}`,
  },
  {
    type: "contract_category",
    input: { ...noise, name: "Insurance" },
    initialValues: { name: "Insurance" },
    dialogTitle: "Create Category",
    shown: ["Insurance"],
    createPath: "/modules/contracts/categories",
    created: { name: "Insurance" },
    entityPath: `/contracts/categories/${ids.created}`,
  },
  {
    type: "purchase_category",
    input: { ...noise, name: "Furniture" },
    initialValues: { name: "Furniture" },
    dialogTitle: "Create Category",
    shown: ["Furniture"],
    createPath: "/modules/purchases/categories",
    created: { name: "Furniture" },
    entityPath: `/purchases/categories/${ids.created}`,
  },
  {
    type: "ledger_category",
    parentId: ids.ledgerCategory,
    input: { ...noise, name: "Groceries", matchWords: ["rewe", "edeka"] },
    initialValues: { name: "Groceries", matchWords: ["rewe", "edeka"] },
    dialogTitle: "Create ledger category",
    shown: ["Groceries", "rewe, edeka"],
    createPath: "/ledger/categories",
    created: { name: "Groceries", parentId: ids.ledgerCategory, matchWords: ["rewe", "edeka"] },
    entityPath: "/ledger/categories",
  },
  {
    type: "ledger_account",
    input: { ...noise, name: "Giro", bank: "ING", iban: "DE89370400440532013000", currency: "EUR" },
    initialValues: { name: "Giro", bank: "ING", iban: "DE89370400440532013000", currency: "EUR" },
    dialogTitle: "Create account",
    shown: ["Giro", "ING", "DE89370400440532013000", "EUR"],
    createPath: "/ledger/accounts",
    created: { name: "Giro", bank: "ING", iban: "DE89370400440532013000", currency: "EUR" },
    entityPath: `/ledger/accounts/${ids.created}`,
  },
]

const allModules: ModuleId[] = ["contracts", "purchases", "auto", "ledger"]

interface InboxServerOptions {
  drafts: Draft[]
  enabledModules?: ModuleId[]
  purchaseCategories?: boolean
}

// The endpoints the inbox page reads, plus create and decision endpoints.
export function inboxRoutes({ drafts, enabledModules = allModules, purchaseCategories = true }: InboxServerOptions) {
  let pending = drafts
  const decide = (call: ApiCall) => {
    const id = call.path.split("/")[2]
    const decided = pending.find((d) => d.id === id)
    pending = pending.filter((d) => d.id !== id)
    return { draft: decided, warnings: [] }
  }
  const created = () => ({ id: ids.created })
  return {
    "GET /settings": { renewalDays: 30, reminderFrequency: "weekly", enabledModules },
    "GET /drafts?status=pending": () => pending,
    "GET /modules/purchases/categories": purchaseCategories
      ? [{ id: ids.purchaseCategory, name: "Electronics", createdAt: stamp, updatedAt: stamp }]
      : [],
    "GET /modules/contracts/categories": [{ id: ids.contractCategory, name: "Internet", createdAt: stamp, updatedAt: stamp }],
    "GET /vehicles": [{ id: ids.vehicle, name: "Golf", createdAt: stamp, updatedAt: stamp }],
    "GET /ledger/categories": [{ id: ids.ledgerCategory, name: "Household", matchWords: [], createdAt: stamp, updatedAt: stamp }],
    ...Object.fromEntries(acceptCases.map((c) => [`POST ${c.createPath}`, created])),
    ...Object.fromEntries(drafts.flatMap((d) => [[`POST /drafts/${d.id}/accept`, decide], [`POST /drafts/${d.id}/reject`, decide]])),
  }
}
