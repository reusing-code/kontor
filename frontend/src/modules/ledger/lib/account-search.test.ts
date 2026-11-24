import { describe, expect, it } from "vitest"
import { ledgerAccountSearch, parseLedgerAccountSearch } from "@/modules/ledger/lib/account-search"

describe("parseLedgerAccountSearch", () => {
  it("keeps a page after the first", () => {
    expect(parseLedgerAccountSearch({ page: 3 })).toEqual({ page: 3 })
    expect(parseLedgerAccountSearch({ page: "4" })).toEqual({ page: 4 })
  })

  it("drops the first page and anything that is not a page", () => {
    for (const page of [1, 0, -2, 2.5, "abc", "", null, undefined, [2]]) {
      expect(parseLedgerAccountSearch({ page })).toEqual({})
    }
    expect(parseLedgerAccountSearch({ other: "x" })).toEqual({})
  })
})

describe("ledgerAccountSearch", () => {
  it("leaves the first page out of the URL", () => {
    expect(ledgerAccountSearch(1)).toEqual({})
    expect(ledgerAccountSearch(2)).toEqual({ page: 2 })
  })
})
