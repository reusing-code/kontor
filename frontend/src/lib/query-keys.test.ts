import { describe, expect, it } from "vitest"
import type { QueryKey } from "@tanstack/react-query"
import { categoryKeys, type CategoryItemKeys } from "@/lib/query-keys"
import { contractKeys } from "@/modules/contracts/keys"
import { purchaseKeys } from "@/modules/purchases/keys"

// Ids chosen to look like the fixed segments of the other keys.
const tricky = ["summary", "list", "detail", "category", "upcoming-renewals"]

function itemKeys(name: string, keys: CategoryItemKeys): [string, QueryKey][] {
  return [
    [`${name}.lists`, keys.lists()],
    [`${name}.summary`, keys.summary()],
    ...tricky.flatMap((id): [string, QueryKey][] => [
      [`${name}.byCategory(${id})`, keys.byCategory(id)],
      [`${name}.detail(${id})`, keys.detail(id)],
    ]),
  ]
}

const named: [string, QueryKey][] = [
  ...itemKeys("contracts", contractKeys),
  ...itemKeys("purchases", purchaseKeys),
  ["contracts.upcomingRenewals(30)", contractKeys.upcomingRenewals(30)],
  ...(["contracts", "purchases"] as const).flatMap((kind): [string, QueryKey][] => [
    [`categories(${kind}).list`, categoryKeys(kind).list()],
    ...tricky.map((id): [string, QueryKey] => [`categories(${kind}).detail(${id})`, categoryKeys(kind).detail(id)]),
  ]),
]

const isPrefix = (prefix: QueryKey, key: QueryKey) =>
  prefix.length <= key.length && prefix.every((part, i) => JSON.stringify(part) === JSON.stringify(key[i]))

// The one intended nesting: the list of a category lies below all lists.
const intended = (a: string, b: string) => a.endsWith(".lists") && b.startsWith(a.replace(".lists", ".byCategory"))

describe("query key factories", () => {
  it("never let one key match another by prefix, apart from lists above the category lists", () => {
    const overlaps = named.flatMap(([a, keyA]) =>
      named.filter(([b, keyB]) => a !== b && isPrefix(keyA, keyB) && !intended(a, b)).map(([b]) => `${a} ⊇ ${b}`),
    )
    expect(overlaps).toEqual([])
  })

  it("keeps every item key below its module root", () => {
    expect(itemKeys("contracts", contractKeys).every(([, key]) => isPrefix(contractKeys.all, key))).toBe(true)
    expect(itemKeys("purchases", purchaseKeys).every(([, key]) => isPrefix(purchaseKeys.all, key))).toBe(true)
    expect(isPrefix(contractKeys.all, contractKeys.upcomingRenewals(30))).toBe(true)
    expect(purchaseKeys.summary()).toEqual(["purchases", "summary"])
  })
})
