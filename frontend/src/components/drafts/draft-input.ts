import type { ClassifiedEntityType, DraftEntityType, DraftTargetType } from "@/types/drafts"
import type { ModuleId } from "@/types/modules"

export const draftEntityTypes: DraftEntityType[] = [
  "purchase",
  "contract",
  "cost",
  "vehicle",
  "contract_category",
  "purchase_category",
  "ledger_category",
  "ledger_account",
]

// What the classifier can propose; quick add only offers these.
export const classifiedEntityTypes: ClassifiedEntityType[] = ["purchase", "contract", "cost"]

export function isClassifiedType(type: DraftTargetType): type is ClassifiedEntityType {
  return (classifiedEntityTypes as DraftTargetType[]).includes(type)
}

export const draftTargetModule: Record<DraftEntityType, ModuleId> = {
  purchase: "purchases",
  contract: "contracts",
  cost: "auto",
  vehicle: "auto",
  contract_category: "contracts",
  purchase_category: "purchases",
  ledger_category: "ledger",
  ledger_account: "ledger",
}

const formFields: Record<DraftEntityType, string[]> = {
  purchase: ["type", "itemName", "brand", "articleNumber", "dealer", "price", "purchaseDate", "descriptionUrl",
    "invoiceUrl", "handbookUrl", "consumables", "comments"],
  contract: ["name", "productName", "company", "contractNumber", "customerNumber", "price", "billingInterval",
    "startDate", "endDate", "minimumDurationMonths", "extensionDurationMonths", "noticePeriodMonths",
    "customerPortalUrl", "comments"],
  cost: ["type", "description", "vendor", "amount", "date", "mileage", "comments"],
  vehicle: ["name", "make", "model", "year", "licensePlate", "purchaseDate", "purchasePrice", "purchaseMileage",
    "targetMileage", "targetMonths", "annualInsurance", "annualTax", "maintenanceFactor", "comments"],
  contract_category: ["name"],
  purchase_category: ["name"],
  ledger_category: ["name", "matchWords"],
  ledger_account: ["name", "bank", "iban", "currency"],
}

// What a draft's parentId refers to.
export type ParentKind = "category" | "vehicle" | "ledgerCategory" | "none"

export function parentKind(type: DraftTargetType): ParentKind {
  switch (type) {
    case "purchase":
    case "contract":
      return "category"
    case "cost":
      return "vehicle"
    case "ledger_category":
      return "ledgerCategory"
    default:
      return "none"
  }
}

// Only entries that live inside a category or vehicle need a parent; a
// ledger category's parent is optional.
export function parentRequired(type: DraftTargetType): boolean {
  return type === "purchase" || type === "contract" || type === "cost"
}

function present(value: unknown): boolean {
  return value !== undefined && value !== null && value !== ""
}

// initialValues picks the draft input fields the target's create dialog knows.
export function initialValues<T>(type: DraftEntityType, input: Record<string, unknown>): Partial<T> {
  const picked: Record<string, unknown> = {}
  for (const key of formFields[type]) {
    if (present(input[key])) picked[key] = input[key]
  }
  return picked as Partial<T>
}

function first(input: Record<string, unknown>, ...keys: string[]): unknown {
  return keys.map((k) => input[k]).find(present)
}

// convertInput carries the common fields (title, party, amount, date) over
// when the user changes a draft's type; the structural types (vehicle,
// categories, account) only carry the name. Switching to "none" keeps the input.
export function convertInput(
  from: DraftTargetType,
  to: DraftTargetType,
  input: Record<string, unknown>,
): Record<string, unknown> {
  if (from === to || to === "none") return input
  const title = first(input, "itemName", "name", "description")
  const party = first(input, "dealer", "company", "vendor")
  const amount = first(input, "price", "amount")
  const date = first(input, "purchaseDate", "startDate", "date")
  let converted: Record<string, unknown>
  switch (to) {
    case "purchase":
      converted = { itemName: title, dealer: party, price: amount, purchaseDate: date }
      break
    case "contract":
      converted = { name: title, company: party, price: amount, startDate: date }
      break
    case "cost":
      converted = { type: "misc", description: title, vendor: party, amount, date }
      break
    default:
      converted = { name: title }
  }
  if (present(input.comments) && parentRequired(to)) converted.comments = input.comments
  return Object.fromEntries(Object.entries(converted).filter(([, v]) => present(v)))
}

export interface DraftOption {
  id: string
  label: string
}

// PrefillEntry is what a create dialog is opened with: the entity type, its
// category or vehicle (or the optional ledger parent category) and the module
// create-input fields.
export interface PrefillEntry {
  type: DraftEntityType
  parentId: string
  input: Record<string, unknown>
}

export function entityPath(type: DraftEntityType, entityId: string, parentId: string): string {
  switch (type) {
    case "purchase":
      return `/purchases/${entityId}`
    case "contract":
      return `/contracts/${entityId}`
    case "cost":
      return `/auto/vehicles/${parentId}`
    case "vehicle":
      return `/auto/vehicles/${entityId}`
    case "contract_category":
      return `/contracts/categories/${entityId}`
    case "purchase_category":
      return `/purchases/categories/${entityId}`
    case "ledger_category":
      return "/ledger/categories"
    case "ledger_account":
      return `/ledger/accounts/${entityId}`
  }
}

export function entityUrl(type: DraftEntityType, entityId: string, parentId: string): string {
  return window.location.origin + entityPath(type, entityId, parentId)
}

export type TargetProblem = "pickType" | "moduleDisabled" | "noCategories" | "noVehicles" | "pickCategory" | "pickVehicle"

// targetProblem tells what is missing before an entry of this type can be
// created in parentId, or null when nothing is.
export function targetProblem(
  target: DraftTargetType,
  parentId: string | undefined,
  enabledTypes: DraftEntityType[],
  parentOptions: Record<DraftEntityType, DraftOption[]>,
): TargetProblem | null {
  if (target === "none") return "pickType"
  if (!enabledTypes.includes(target)) return "moduleDisabled"
  if (!parentRequired(target)) return null
  const options = parentOptions[target]
  if (options.length === 0) return target === "cost" ? "noVehicles" : "noCategories"
  if (!options.some((o) => o.id === parentId)) return target === "cost" ? "pickVehicle" : "pickCategory"
  return null
}
