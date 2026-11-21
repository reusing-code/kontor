import { describe, expect, it } from "vitest"
import { parseAmountInput, rowsFromSplits, splitProblem, splitRemainder, splitsFromRows, type SplitRow } from "@/modules/ledger/lib/splits"

const row = (amount: string, categoryId = "cat", note = ""): SplitRow => ({ amount, categoryId, note })

describe("parseAmountInput", () => {
  it("reads whole and decimal amounts with comma or dot", () => {
    expect(parseAmountInput("150")).toBe(15000)
    expect(parseAmountInput(" 150,5 ")).toBe(15050)
    expect(parseAmountInput("150.05")).toBe(15005)
  })

  it("refuses zero, signs, thousands separators and more than two decimals", () => {
    for (const text of ["", "0", "0,00", "-5", "1.234,56", "1,234", "abc"]) {
      expect(parseAmountInput(text)).toBeUndefined()
    }
  })
})

describe("splitProblem", () => {
  it("accepts parts below the transaction amount", () => {
    expect(splitProblem([], -17235)).toBeNull()
    expect(splitProblem([row("150")], -17235)).toBeNull()
  })

  it("names the first problem", () => {
    expect(splitProblem([row("x")], -17235)).toBe("amount")
    expect(splitProblem([row("150", "")], -17235)).toBe("category")
    expect(splitProblem([row("150", "cat", "x".repeat(501))], -17235)).toBe("note")
    expect(splitProblem([row("100"), row("72,35")], -17235)).toBe("total")
    expect(splitProblem(Array.from({ length: 11 }, () => row("1")), -17235)).toBe("tooMany")
  })
})

describe("splitRemainder and splitsFromRows", () => {
  it("apply the transaction's sign", () => {
    const rows = [row("150", "cash", " Auszahlung ")]
    expect(splitRemainder(rows, -17235)).toBe(-2235)
    expect(splitRemainder(rows, 20000)).toBe(5000)
    expect(splitsFromRows(rows, -17235)).toEqual([{ amountMinor: -15000, categoryId: "cash", note: "Auszahlung" }])
    expect(splitsFromRows([row("5")], 20000)).toEqual([{ amountMinor: 500, categoryId: "cat" }])
  })

  it("round-trip stored parts", () => {
    expect(rowsFromSplits([{ amountMinor: -15000, categoryId: "cash" }, { amountMinor: -5, note: "x" }], "de")).toEqual([
      { amount: "150,00", categoryId: "cash", note: "" },
      { amount: "0,05", categoryId: "", note: "x" },
    ])
  })
})
