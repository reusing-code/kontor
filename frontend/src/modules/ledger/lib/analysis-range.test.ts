import { describe, expect, it, vi } from "vitest"
import { todayISO } from "@/modules/ledger/lib/balance"
import {
  analysisQuery,
  customRangeProblem,
  defaultAnalysisRange,
  formatMonth,
  presetRange,
  resolveRange,
} from "@/modules/ledger/lib/analysis-range"

describe("presetRange", () => {
  it("counts the current month as one of the N months", () => {
    expect(presetRange("6m", "2026-11-22")).toEqual({ from: "2026-06-01", to: "2026-11-22" })
    expect(presetRange("12m", "2026-11-22")).toEqual({ from: "2025-12-01", to: "2026-11-22" })
  })

  it("crosses the year in January", () => {
    expect(presetRange("6m", "2026-01-15")).toEqual({ from: "2025-08-01", to: "2026-01-15" })
    expect(presetRange("12m", "2026-01-15")).toEqual({ from: "2025-02-01", to: "2026-01-15" })
    expect(presetRange("12m", "2026-12-31")).toEqual({ from: "2026-01-01", to: "2026-12-31" })
  })

  it("starts on the first of a month from a 31st", () => {
    expect(presetRange("6m", "2026-07-31")).toEqual({ from: "2026-02-01", to: "2026-07-31" })
    expect(presetRange("6m", "2026-03-31")).toEqual({ from: "2025-10-01", to: "2026-03-31" })
  })

  it("year to date starts on 1 January", () => {
    expect(presetRange("ytd", "2026-11-22")).toEqual({ from: "2026-01-01", to: "2026-11-22" })
    expect(presetRange("ytd", "2026-01-01")).toEqual({ from: "2026-01-01", to: "2026-01-01" })
  })

  it("all leaves both ends to the server", () => {
    expect(presetRange("all", "2026-11-22")).toEqual({})
  })
})

describe("customRangeProblem", () => {
  const today = "2026-11-22"

  it("accepts a mid-month range up to today", () => {
    expect(customRangeProblem("2026-03-17", "2026-11-22", today)).toBeUndefined()
    expect(customRangeProblem("2026-03-17", "2026-03-17", today)).toBeUndefined()
  })

  it("names what is wrong", () => {
    expect(customRangeProblem("", "2026-11-01", today)).toBe("incomplete")
    expect(customRangeProblem("2026-03-17", "", today)).toBe("incomplete")
    expect(customRangeProblem("2026-02-30", "2026-11-01", today)).toBe("incomplete")
    expect(customRangeProblem("2026-11-02", "2026-11-01", today)).toBe("fromAfterTo")
    expect(customRangeProblem("2026-11-02", "2026-11-23", today)).toBe("toInFuture")
  })

  it("refuses more months than the server accepts", () => {
    expect(customRangeProblem("1926-12-01", "2026-11-22", today)).toBeUndefined()
    expect(customRangeProblem("1926-11-30", "2026-11-22", today)).toBe("tooLong")
    // what a date input reports while the year 2025 is typed
    for (const typing of ["0002-12-01", "0020-12-01", "0202-12-01"]) {
      expect(customRangeProblem(typing, "2026-11-22", today)).toBe("tooLong")
    }
    expect(customRangeProblem("2025-12-01", "2026-11-22", today)).toBeUndefined()
  })
})

describe("resolveRange", () => {
  it("gives the range of a preset and of a valid custom range unchanged", () => {
    expect(resolveRange({ kind: "preset", preset: "ytd" }, "2026-11-22")).toEqual({ range: { from: "2026-01-01", to: "2026-11-22" } })
    expect(resolveRange({ kind: "custom", from: "2026-03-17", to: "2026-04-09" }, "2026-11-22")).toEqual({
      range: { from: "2026-03-17", to: "2026-04-09" },
    })
  })

  it("gives the problem of an invalid custom range and no range", () => {
    expect(resolveRange({ kind: "custom", from: "2026-05-01", to: "2026-04-09" }, "2026-11-22")).toEqual({ problem: "fromAfterTo" })
  })

  it("defaults to the last 12 months of the local date", () => {
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date(2026, 0, 31, 12, 0, 0))
    expect(todayISO()).toBe("2026-01-31")
    expect(resolveRange(defaultAnalysisRange, todayISO())).toEqual({ range: { from: "2025-02-01", to: "2026-01-31" } })
  })
})

describe("analysisQuery", () => {
  it("omits empty values", () => {
    expect(analysisQuery({})).toBe("")
    expect(analysisQuery({ from: "2026-01-01", to: "2026-11-22" })).toBe("?from=2026-01-01&to=2026-11-22")
    expect(analysisQuery({ to: "2026-11-22" }, { parentCategoryId: undefined })).toBe("?to=2026-11-22")
    expect(analysisQuery({}, { parentCategoryId: "cat-1" })).toBe("?parentCategoryId=cat-1")
  })
})

describe("formatting", () => {
  it("labels a month", () => {
    expect(formatMonth("2026-03", "en")).toBe("Mar 26")
    expect(formatMonth("2025-12", "en")).toBe("Dec 25")
    expect(formatMonth("2026-01", "de")).toMatch(/^Jan\.? 26$/)
  })
})
