import type { DraftEntityType, DraftTargetType } from "@/types/drafts"
import type { ModuleId } from "@/types/modules"

export const draftEntityTypes: DraftEntityType[] = ["purchase", "contract", "cost"]

export const draftTargetModule: Record<DraftEntityType, ModuleId> = {
  purchase: "purchases",
  contract: "contracts",
  cost: "auto",
}

const formFields: Record<DraftEntityType, string[]> = {
  purchase: ["type", "itemName", "brand", "articleNumber", "dealer", "price", "purchaseDate", "descriptionUrl",
    "invoiceUrl", "handbookUrl", "consumables", "comments"],
  contract: ["name", "productName", "company", "contractNumber", "customerNumber", "price", "billingInterval",
    "startDate", "endDate", "minimumDurationMonths", "extensionDurationMonths", "noticePeriodMonths",
    "customerPortalUrl", "comments"],
  cost: ["type", "description", "vendor", "amount", "date", "mileage", "comments"],
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
// when the user changes a draft's type. Switching to "none" keeps the input.
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
  const converted: Record<string, unknown> =
    to === "purchase"
      ? { itemName: title, dealer: party, price: amount, purchaseDate: date }
      : to === "contract"
        ? { name: title, company: party, price: amount, startDate: date }
        : { type: "misc", description: title, vendor: party, amount, date }
  if (present(input.comments)) converted.comments = input.comments
  return Object.fromEntries(Object.entries(converted).filter(([, v]) => present(v)))
}

export interface DraftOption {
  id: string
  label: string
}

// PrefillEntry is what a create dialog is opened with: the entity type, its
// category or vehicle and the module create-input fields.
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
  const options = parentOptions[target]
  if (options.length === 0) return target === "cost" ? "noVehicles" : "noCategories"
  if (!options.some((o) => o.id === parentId)) return target === "cost" ? "pickVehicle" : "pickCategory"
  return null
}
