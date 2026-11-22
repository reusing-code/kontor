import { describe, expect, it } from "vitest"
import { analysisQuery, customRangeProblem, presetRange, resolveRange } from "@/modules/ledger/lib/analysis-range"

describe("presetRange at the edges of the calendar", () => {
  it.each([
    ["2024-02-29", "2023-09-01", "2023-03-01", "2024-01-01"],
    ["2026-01-01", "2025-08-01", "2025-02-01", "2026-01-01"],
    ["2026-12-31", "2026-07-01", "2026-01-01", "2026-01-01"],
    ["2026-06-30", "2026-01-01", "2025-07-01", "2026-01-01"],
    ["2000-01-31", "1999-08-01", "1999-02-01", "2000-01-01"],
  ])("today %s", (today, sixMonths, twelveMonths, yearToDate) => {
    expect(presetRange("6m", today)).toEqual({ from: sixMonths, to: today })
    expect(presetRange("12m", today)).toEqual({ from: twelveMonths, to: today })
    expect(presetRange("ytd", today)).toEqual({ from: yearToDate, to: today })
  })

  it("never gives a range the custom check would refuse", () => {
    for (const today of ["2024-02-29", "2026-01-01", "2026-12-31", "2026-03-31"]) {
      for (const preset of ["6m", "12m", "ytd"] as const) {
        const { from = "", to = "" } = presetRange(preset, today)
        expect(customRangeProblem(from, to, today)).toBeUndefined()
      }
    }
  })
})

describe("customRangeProblem at the boundaries", () => {
  const today = "2026-11-22"

  it("accepts today as both ends and a leap day", () => {
    expect(customRangeProblem(today, today, today)).toBeUndefined()
    expect(customRangeProblem("2024-02-29", "2024-02-29", today)).toBeUndefined()
  })

  it.each([
    ["2025-02-29", today],
    ["2026-04-31", today],
    ["2026-13-01", today],
    ["2026-00-10", today],
    ["2026-1-5", today],
    ["26-01-05", today],
    ["2026-01-05T00:00:00Z", today],
    [" 2026-01-05", today],
    ["2026-01-05", "tomorrow"],
    ["<script>", "2026-11-01"],
    ["+002026-01-05", today],
  ])("treats %s .. %s as incomplete", (from, to) => {
    expect(customRangeProblem(from, to, today)).toBe("incomplete")
    expect(resolveRange({ kind: "custom", from, to }, today)).toEqual({ problem: "incomplete" })
  })

  it("reports the order before the future and the length", () => {
    expect(customRangeProblem("2027-01-02", "2027-01-01", today)).toBe("fromAfterTo")
    expect(customRangeProblem("0001-01-01", "2026-11-23", today)).toBe("toInFuture")
    expect(customRangeProblem("2026-11-23", "2026-11-23", today)).toBe("toInFuture")
  })
})

describe("analysisQuery", () => {
  it("encodes what it is given", () => {
    const query = analysisQuery({ from: "2026-01-01", to: "2026-01-31" }, { parentCategoryId: "a&b=c d#e" })
    expect(query).toBe("?from=2026-01-01&to=2026-01-31&parentCategoryId=a%26b%3Dc+d%23e")
    expect(new URLSearchParams(query).get("parentCategoryId")).toBe("a&b=c d#e")
  })
})
