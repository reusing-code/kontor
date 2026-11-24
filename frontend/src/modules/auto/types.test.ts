import { describe, expect, it } from "vitest"
import { costEntryFormSchema, vehicleFormSchema } from "@/modules/auto/types"
import { issuePaths } from "@/test/schema"

describe("vehicleFormSchema", () => {
  it("takes a vehicle with only a name and turns the number inputs into numbers", () => {
    expect(issuePaths(vehicleFormSchema, { name: "Golf" })).toEqual([])
    expect(vehicleFormSchema.parse({ name: "Golf", year: "2020", purchasePrice: "15000.50", targetMonths: "" })).toMatchObject({
      year: 2020, purchasePrice: 15000.5, targetMonths: undefined,
    })
  })

  it.each([
    ["name", { name: "" }],
    ["year", { year: "2020.5" }],
    ["purchaseDate", { purchaseDate: "yesterday" }],
    ["purchasePrice", { purchasePrice: "-1" }],
    ["purchaseMileage", { purchaseMileage: "abc" }],
    ["targetMileage", { targetMileage: -1 }],
    ["targetMonths", { targetMonths: "1.5" }],
    ["annualInsurance", { annualInsurance: "-5" }],
    ["annualTax", { annualTax: "x" }],
    ["maintenanceFactor", { maintenanceFactor: "-0.1" }],
  ])("refuses an invalid %s", (path, change) => {
    expect(issuePaths(vehicleFormSchema, { name: "Golf", ...change })).toEqual([path])
  })
})

describe("costEntryFormSchema", () => {
  const minimal = { type: "fuel", date: "2026-11-01" }

  it("takes a cost entry with type and date", () => {
    expect(issuePaths(costEntryFormSchema, minimal)).toEqual([])
    expect(costEntryFormSchema.parse({ ...minimal, amount: "60.10", mileage: "12000" })).toMatchObject({ amount: 60.1, mileage: 12000 })
  })

  it.each([
    ["type", { type: "parking" }],
    ["type", { type: undefined }],
    ["date", { date: undefined }],
    ["date", { date: "2026-11-31" }],
    ["amount", { amount: "-1" }],
    ["mileage", { mileage: "far" }],
  ])("refuses an invalid %s", (path, change) => {
    expect(issuePaths(costEntryFormSchema, { ...minimal, ...change })).toEqual([path])
  })
})
