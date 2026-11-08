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

export function entityUrl(type: DraftEntityType, entityId: string, parentId: string): string {
  const origin = window.location.origin
  switch (type) {
    case "purchase":
      return `${origin}/purchases/${entityId}`
    case "contract":
      return `${origin}/contracts/${entityId}`
    case "cost":
      return `${origin}/auto/vehicles/${parentId}`
  }
}
