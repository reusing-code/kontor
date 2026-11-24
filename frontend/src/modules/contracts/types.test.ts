import { describe, expect, it } from "vitest"
import { contractFormSchema } from "@/modules/contracts/types"
import { issuePaths } from "@/test/schema"

const minimal = { name: "Gym", billingInterval: "monthly", startDate: "2026-01-01" }

describe("contractFormSchema", () => {
  it("takes a minimal contract and turns the number inputs into numbers", () => {
    expect(issuePaths(contractFormSchema, minimal)).toEqual([])
    expect(contractFormSchema.parse({ ...minimal, price: "29.90", noticePeriodMonths: "3", minimumDurationMonths: "" })).toMatchObject({
      price: 29.9, noticePeriodMonths: 3, minimumDurationMonths: undefined,
    })
  })

  it.each([
    ["name", { name: "" }],
    ["billingInterval", { billingInterval: "weekly" }],
    ["startDate", { startDate: undefined }],
    ["startDate", { startDate: "01.01.2026" }],
    ["endDate", { endDate: "2026-02-30" }],
    ["price", { price: "-1" }],
    ["price", { price: "abc" }],
    ["noticePeriodMonths", { noticePeriodMonths: "1.5" }],
    ["minimumDurationMonths", { minimumDurationMonths: -12 }],
    ["extensionDurationMonths", { extensionDurationMonths: "x" }],
    ["customerPortalUrl", { customerPortalUrl: "portal.example" }],
    ["paperlessUrl", { paperlessUrl: "not a url" }],
  ])("refuses an invalid %s", (path, change) => {
    expect(issuePaths(contractFormSchema, { ...minimal, ...change })).toEqual([path])
  })

  it("accepts empty URLs", () => {
    expect(issuePaths(contractFormSchema, { ...minimal, customerPortalUrl: "", paperlessUrl: "" })).toEqual([])
  })
})
