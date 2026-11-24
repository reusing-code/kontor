import { describe, expect, it } from "vitest"
import { parseAllTransactionsSearch, selectedAccountIds, toggleAccount } from "@/modules/ledger/lib/transaction-filter"

describe("parseAllTransactionsSearch", () => {
  it("keeps distinct non-empty string IDs", () => {
    expect(parseAllTransactionsSearch({ accounts: ["a", "b", "a", "", 3] })).toEqual({ accounts: ["a", "b"] })
  })

  it("means all accounts without a usable selection", () => {
    expect(parseAllTransactionsSearch({})).toEqual({})
    expect(parseAllTransactionsSearch({ accounts: [] })).toEqual({})
    expect(parseAllTransactionsSearch({ accounts: "a" })).toEqual({})
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
