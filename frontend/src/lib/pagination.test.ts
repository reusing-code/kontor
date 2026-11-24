import { describe, expect, it } from "vitest"
import { pageCountOf, pageItems } from "@/lib/pagination"

describe("pageItems", () => {
  it("lists every page when there are few", () => {
    expect(pageItems(1, 1)).toEqual([1])
    expect(pageItems(2, 5)).toEqual([1, 2, 3, 4, 5])
  })

  it("puts ellipses into gaps of two or more pages", () => {
    expect(pageItems(1, 20)).toEqual([1, 2, "ellipsis", 20])
    expect(pageItems(10, 20)).toEqual([1, "ellipsis", 9, 10, 11, "ellipsis", 20])
    expect(pageItems(20, 20)).toEqual([1, "ellipsis", 19, 20])
  })

  it("shows a single skipped page instead of an ellipsis", () => {
    expect(pageItems(4, 20)).toEqual([1, 2, 3, 4, 5, "ellipsis", 20])
    expect(pageItems(17, 20)).toEqual([1, "ellipsis", 16, 17, 18, 19, 20])
  })

  it("clamps a page out of range", () => {
    expect(pageItems(0, 3)).toEqual([1, 2, 3])
    expect(pageItems(9, 3)).toEqual([1, 2, 3])
    expect(pageItems(1, 0)).toEqual([])
  })
})

describe("pageCountOf", () => {
  it("has at least one page", () => {
    expect(pageCountOf(0, 100)).toBe(1)
    expect(pageCountOf(100, 100)).toBe(1)
    expect(pageCountOf(101, 100)).toBe(2)
  })
})
