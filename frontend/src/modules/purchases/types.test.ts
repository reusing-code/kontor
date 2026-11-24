import { describe, expect, it } from "vitest"
import { purchaseFormSchema } from "@/modules/purchases/types"
import { issuePaths } from "@/test/schema"

describe("purchaseFormSchema", () => {
  it("takes a purchase with only a name", () => {
    expect(issuePaths(purchaseFormSchema, { itemName: "Lamp" })).toEqual([])
    expect(purchaseFormSchema.parse({ itemName: "Lamp", price: "12.5" })).toMatchObject({ price: 12.5 })
    expect(purchaseFormSchema.parse({ itemName: "Lamp", price: "" }).price).toBeUndefined()
  })

  it.each([
    ["itemName", { itemName: "" }],
    ["itemName", { itemName: undefined }],
    ["price", { price: "-0.01" }],
    ["purchaseDate", { purchaseDate: "2026-13-01" }],
    ["descriptionUrl", { descriptionUrl: "shop" }],
    ["invoiceUrl", { invoiceUrl: "invoice.pdf" }],
    ["handbookUrl", { handbookUrl: "manual" }],
  ])("refuses an invalid %s", (path, change) => {
    expect(issuePaths(purchaseFormSchema, { itemName: "Lamp", ...change })).toEqual([path])
  })

  it("accepts empty URLs", () => {
    expect(issuePaths(purchaseFormSchema, { itemName: "Lamp", descriptionUrl: "", invoiceUrl: "", handbookUrl: "" })).toEqual([])
  })
})
