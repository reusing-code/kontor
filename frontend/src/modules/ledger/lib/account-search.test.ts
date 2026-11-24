import { describe, expect, it } from "vitest"
import { ledgerAccountSearch, parseLedgerAccountSearch } from "@/modules/ledger/lib/account-search"

describe("parseLedgerAccountSearch", () => {
  it("keeps a page after the first and nothing else", () => {
    expect(parseLedgerAccountSearch({ page: 3, other: "x" })).toEqual({ page: 3 })
    expect(parseLedgerAccountSearch({ page: 1 })).toEqual({})
    expect(parseLedgerAccountSearch({ page: "abc" })).toEqual({})
  })
})

describe("ledgerAccountSearch", () => {
  it("leaves the first page out of the URL", () => {
    expect(ledgerAccountSearch(1)).toEqual({})
    expect(ledgerAccountSearch(2)).toEqual({ page: 2 })
  })
})
