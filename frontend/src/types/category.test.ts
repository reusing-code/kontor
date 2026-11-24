import { describe, expect, it } from "vitest"
import { categoryFormSchema } from "@/types/category"
import { issuePaths } from "@/test/schema"

describe("categoryFormSchema", () => {
  it("needs a name", () => {
    expect(issuePaths(categoryFormSchema, { name: "Insurance" })).toEqual([])
    expect(issuePaths(categoryFormSchema, { name: "" })).toEqual(["name"])
    expect(issuePaths(categoryFormSchema, {})).toEqual(["name"])
  })
})
