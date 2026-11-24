import { describe, expect, it } from "vitest"
import {
  convertInput,
  draftEntityTypes,
  entityPath,
  entityUrl,
  initialValues,
  isClassifiedType,
  parentKind,
  parentRequired,
  targetProblem,
  type DraftOption,
} from "@/components/drafts/draft-input"
import type { DraftEntityType } from "@/types/drafts"

function options(overrides: Partial<Record<DraftEntityType, DraftOption[]>> = {}): Record<DraftEntityType, DraftOption[]> {
  const empty = Object.fromEntries(draftEntityTypes.map((type) => [type, []])) as unknown as Record<DraftEntityType, DraftOption[]>
  return { ...empty, ...overrides }
}

const category = { id: "cat-1", label: "Home" }
const vehicle = { id: "veh-1", label: "Golf" }

describe("initialValues", () => {
  it("keeps the fields the purchase dialog knows and drops the rest and empty values", () => {
    expect(initialValues("purchase", {
      itemName: "Lamp",
      dealer: "Shop",
      price: 29.99,
      purchaseDate: "2026-11-01",
      brand: "",
      comments: null,
      company: "Not a purchase field",
      vendor: "Neither",
    })).toEqual({ itemName: "Lamp", dealer: "Shop", price: 29.99, purchaseDate: "2026-11-01" })
  })

  it("keeps zero, which is a value", () => {
    expect(initialValues("contract", { name: "Gym", price: 0, noticePeriodMonths: 0 })).toEqual({ name: "Gym", price: 0, noticePeriodMonths: 0 })
  })

  it.each([
    ["contract", { name: "Gym", company: "FitCo", billingInterval: "monthly", startDate: "2026-01-01", itemName: "x" },
      { name: "Gym", company: "FitCo", billingInterval: "monthly", startDate: "2026-01-01" }],
    ["cost", { type: "fuel", amount: 50, date: "2026-11-01", mileage: 12000, name: "x" },
      { type: "fuel", amount: 50, date: "2026-11-01", mileage: 12000 }],
    ["vehicle", { name: "Golf", make: "VW", year: 2020, licensePlate: "B-X 1", amount: 3 },
      { name: "Golf", make: "VW", year: 2020, licensePlate: "B-X 1" }],
    ["contract_category", { name: "Insurance", matchWords: ["x"] }, { name: "Insurance" }],
    ["purchase_category", { name: "Tools", price: 3 }, { name: "Tools" }],
    ["ledger_category", { name: "Food", matchWords: ["aldi"], parentId: "p" }, { name: "Food", matchWords: ["aldi"] }],
    ["ledger_account", { name: "Giro", bank: "DKB", iban: "DE00", currency: "EUR", balance: 1 },
      { name: "Giro", bank: "DKB", iban: "DE00", currency: "EUR" }],
  ] as const)("picks the %s fields", (type, input, expected) => {
    expect(initialValues(type, input)).toEqual(expected)
  })
})

describe("convertInput", () => {
  const purchase = { itemName: "Lamp", dealer: "Shop", price: 30, purchaseDate: "2026-11-01", comments: "gift", brand: "Acme" }

  it("carries title, party, amount and date between the entry types", () => {
    expect(convertInput("purchase", "contract", purchase)).toEqual({
      name: "Lamp", company: "Shop", price: 30, startDate: "2026-11-01", comments: "gift",
    })
    expect(convertInput("purchase", "cost", purchase)).toEqual({
      type: "misc", description: "Lamp", vendor: "Shop", amount: 30, date: "2026-11-01", comments: "gift",
    })
    expect(convertInput("cost", "purchase", { type: "fuel", description: "Fuel", vendor: "Aral", amount: 60, date: "2026-10-01" })).toEqual({
      itemName: "Fuel", dealer: "Aral", price: 60, purchaseDate: "2026-10-01",
    })
  })

  it("carries only the name to the structural types", () => {
    for (const to of ["vehicle", "contract_category", "purchase_category", "ledger_category", "ledger_account"] as const) {
      expect(convertInput("purchase", to, purchase)).toEqual({ name: "Lamp" })
    }
  })

  it("keeps the input for the same type and for none", () => {
    expect(convertInput("purchase", "purchase", purchase)).toBe(purchase)
    expect(convertInput("purchase", "none", purchase)).toBe(purchase)
  })

  it("drops fields that have no value", () => {
    expect(convertInput("vehicle", "contract", { name: "Golf" })).toEqual({ name: "Golf" })
    expect(convertInput("contract", "purchase", { company: "", price: 0 })).toEqual({ price: 0 })
  })
})

describe("parentKind and parentRequired", () => {
  it.each([
    ["purchase", "category", true],
    ["contract", "category", true],
    ["cost", "vehicle", true],
    ["ledger_category", "ledgerCategory", false],
    ["vehicle", "none", false],
    ["contract_category", "none", false],
    ["purchase_category", "none", false],
    ["ledger_account", "none", false],
    ["none", "none", false],
  ] as const)("%s has parent %s (required: %s)", (type, kind, required) => {
    expect(parentKind(type)).toBe(kind)
    expect(parentRequired(type)).toBe(required)
  })

  it("knows the classified types", () => {
    expect(["purchase", "contract", "cost", "vehicle", "none"].filter((type) => isClassifiedType(type as DraftEntityType)))
      .toEqual(["purchase", "contract", "cost"])
  })
})

describe("targetProblem", () => {
  const enabled = [...draftEntityTypes]

  it("needs a type", () => {
    expect(targetProblem("none", undefined, enabled, options())).toBe("pickType")
  })

  it("refuses a type of a disabled module, also with a valid parent", () => {
    const only = draftEntityTypes.filter((type) => type !== "purchase")
    expect(targetProblem("purchase", category.id, only, options({ purchase: [category] }))).toBe("moduleDisabled")
  })

  it("asks for a category or a vehicle that exists", () => {
    expect(targetProblem("purchase", undefined, enabled, options())).toBe("noCategories")
    expect(targetProblem("contract", undefined, enabled, options())).toBe("noCategories")
    expect(targetProblem("cost", undefined, enabled, options())).toBe("noVehicles")
    expect(targetProblem("purchase", undefined, enabled, options({ purchase: [category] }))).toBe("pickCategory")
    expect(targetProblem("contract", "gone", enabled, options({ contract: [category] }))).toBe("pickCategory")
    expect(targetProblem("cost", "gone", enabled, options({ cost: [vehicle] }))).toBe("pickVehicle")
  })

  it("is fine with a listed parent", () => {
    expect(targetProblem("purchase", category.id, enabled, options({ purchase: [category] }))).toBeNull()
    expect(targetProblem("cost", vehicle.id, enabled, options({ cost: [vehicle] }))).toBeNull()
  })

  it("does not check the parent of types without a required one", () => {
    for (const type of ["vehicle", "contract_category", "purchase_category", "ledger_category", "ledger_account"] as const) {
      expect(targetProblem(type, "anything", enabled, options())).toBeNull()
      expect(targetProblem(type, undefined, enabled, options())).toBeNull()
    }
  })
})

describe("entityPath and entityUrl", () => {
  it.each([
    ["purchase", "/purchases/e1"],
    ["contract", "/contracts/e1"],
    ["cost", "/auto/vehicles/p1"],
    ["vehicle", "/auto/vehicles/e1"],
    ["contract_category", "/contracts/categories/e1"],
    ["purchase_category", "/purchases/categories/e1"],
    ["ledger_category", "/ledger/categories"],
    ["ledger_account", "/ledger/accounts/e1"],
  ] as const)("a %s opens at %s", (type, path) => {
    expect(entityPath(type, "e1", "p1")).toBe(path)
    expect(entityUrl(type, "e1", "p1")).toBe(`${window.location.origin}${path}`)
  })
})
