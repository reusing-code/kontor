import { describe, expect, it } from "vitest"
import { allTransactionsSearch, parseAllTransactionsSearch, selectedAccountIds, toggleAccount } from "@/modules/ledger/lib/transaction-filter"

describe("parseAllTransactionsSearch", () => {
  it("keeps distinct non-empty string IDs", () => {
    expect(parseAllTransactionsSearch({ accounts: ["a", "b", "a", "", 3] })).toEqual({ accounts: ["a", "b"] })
  })

  it("means all accounts without a usable selection", () => {
    expect(parseAllTransactionsSearch({})).toEqual({})
    expect(parseAllTransactionsSearch({ accounts: [] })).toEqual({})
    expect(parseAllTransactionsSearch({ accounts: "a" })).toEqual({})
  })

  it("keeps a page after the first", () => {
    expect(parseAllTransactionsSearch({ accounts: ["a"], page: 3 })).toEqual({ accounts: ["a"], page: 3 })
    expect(parseAllTransactionsSearch({ page: 1 })).toEqual({})
    expect(parseAllTransactionsSearch({ page: "x" })).toEqual({})
  })
})

describe("allTransactionsSearch", () => {
  it("leaves all accounts and the first page out of the URL", () => {
    expect(allTransactionsSearch(undefined, 1)).toEqual({})
    expect(allTransactionsSearch([], 2)).toEqual({ page: 2 })
    expect(allTransactionsSearch(["a"], 1)).toEqual({ accounts: ["a"] })
  })
})

describe("selectedAccountIds", () => {
  it("drops IDs of unknown accounts", () => {
    expect(selectedAccountIds(["a", "gone"], ["a", "b"])).toEqual(["a"])
    expect(selectedAccountIds(undefined, ["a"])).toEqual([])
  })
})

describe("toggleAccount", () => {
  it("adds and removes an account", () => {
    expect(toggleAccount(["a"], "b")).toEqual(["a", "b"])
    expect(toggleAccount(["a", "b"], "a")).toEqual(["b"])
  })
})
