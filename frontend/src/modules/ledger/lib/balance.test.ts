import { describe, expect, it } from "vitest"
import { hasDrift, parseBalanceInput, todayISO } from "@/modules/ledger/lib/balance"

describe("parseBalanceInput", () => {
  it.each([
    ["1234", 123400],
    ["1234,5", 123450],
    ["1234.56", 123456],
    ["-12,30", -1230],
    ["+7", 700],
    ["0", 0],
    ["-0", 0],
    [" 1 234,00 ", 123400],
  ])("reads %j", (text, want) => {
    expect(parseBalanceInput(text)).toBe(want)
  })

  it.each(["", "abc", "1.234,56", "12,345", "--1", "99999999999999"])("refuses %j", (text) => {
    expect(parseBalanceInput(text)).toBeUndefined()
  })
})

describe("todayISO", () => {
  it("uses the local date", () => {
    expect(todayISO(new Date(2026, 10, 3, 23, 59))).toBe("2026-11-03")
  })
})

describe("hasDrift", () => {
  it("is only true for a non-zero drift", () => {
    expect(hasDrift(undefined)).toBe(false)
    expect(hasDrift(0)).toBe(false)
    expect(hasDrift(-1)).toBe(true)
  })
})
