import { describe, expect, it } from "vitest"
import type { LedgerReviewQueueItem } from "@/modules/ledger/types"
import { batchReviewItems, categoryOverrides, counterpartyKey, groupReviewQueue, reviewProposal } from "@/modules/ledger/lib/review-queue"

let seq = 0
function txn(fields: Partial<LedgerReviewQueueItem>): LedgerReviewQueueItem {
  seq += 1
  const at = "2026-11-01T10:00:00Z"
  return {
    id: `00000000-0000-4000-8000-${String(seq).padStart(12, "0")}`,
    accountId: "00000000-0000-4000-8000-aaaaaaaaaaaa",
    bookingDate: "2026-11-01",
    amountMinor: -1000,
    currency: "EUR",
    reviewStatus: "needsReview",
    categorizationSource: "none",
    sourceType: "dkb.csv",
    importBatchId: "00000000-0000-4000-8000-bbbbbbbbbbbb",
    fingerprint: `fp-${seq}`,
    createdAt: at,
    updatedAt: at,
    ...fields,
  }
}

const groceries = "00000000-0000-4000-8000-000000000a01"
const fuel = "00000000-0000-4000-8000-000000000a02"

describe("counterpartyKey", () => {
  it("drops numbers, short words, stop words and repeats", () => {
    expect(counterpartyKey("ALDI SAGT DANKE 1234")).toBe("aldi")
    expect(counterpartyKey("Aldi Süd GmbH, Filiale 0815")).toBe("aldi süd filiale")
    expect(counterpartyKey("REWE Markt REWE")).toBe("rewe markt")
    expect(counterpartyKey("DB 12")).toBe("")
    expect(counterpartyKey(undefined)).toBe("")
  })
})

describe("reviewProposal", () => {
  it("proposes an assigned category, an LLM suggestion only with the LLM, and nothing for transfers", () => {
    expect(reviewProposal(txn({ categoryId: groceries, categorizationSource: "keyword" }), false)).toEqual({ categoryId: groceries, kind: "assigned" })
    const suggested = txn({ suggestion: { categoryId: fuel, confidence: "medium", matchWord: "aral" } })
    expect(reviewProposal(suggested, true)).toEqual({ categoryId: fuel, kind: "suggested", confidence: "medium", matchWord: "aral" })
    expect(reviewProposal(suggested, false)).toBeUndefined()
    expect(reviewProposal(txn({ categoryId: groceries, transferPairTransactionId: groceries }), true)).toBeUndefined()
    expect(reviewProposal(txn({}), true)).toBeUndefined()
  })
})

describe("groupReviewQueue", () => {
  it("groups the same counterparty and proposal at the position of the newest member", () => {
    const shell = txn({ counterpartyName: "Shell 4711", suggestion: { categoryId: fuel, confidence: "high" } })
    const aldi1 = txn({ counterpartyName: "ALDI SAGT DANKE 1", categoryId: groceries, categorizationSource: "keyword" })
    const plain = txn({ counterpartyName: "Someone" })
    const aldi2 = txn({ counterpartyName: "ALDI SAGT DANKE 2", categoryId: groceries, categorizationSource: "keyword" })
    const aldiOther = txn({ counterpartyName: "ALDI", categoryId: fuel, categorizationSource: "keyword" })

    const entries = groupReviewQueue([shell, aldi1, plain, aldi2, aldiOther], true)

    expect(entries.map((e) => (e.kind === "group" ? e.items.map((i) => i.id) : e.item.id))).toEqual([
      shell.id,
      [aldi1.id, aldi2.id],
      plain.id,
      aldiOther.id,
    ])
  })

  it("keeps assigned and suggested proposals apart and reports the weakest confidence and the common match word", () => {
    const a = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high", matchWord: "aral" } })
    const b = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "low", matchWord: "aral" } })
    const c = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high" } })
    const d = txn({ counterpartyName: "Aral", categoryId: fuel, categorizationSource: "keyword" })

    const [group, single] = groupReviewQueue([a, b, c, d], true)

    expect(group).toMatchObject({ kind: "group", proposal: { kind: "suggested", confidence: "low", matchWord: "aral" } })
    expect(single).toEqual({ kind: "single", item: d })
  })

  it("does not group suggestions without the LLM", () => {
    const a = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high" } })
    const b = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high" } })
    expect(groupReviewQueue([a, b], false).map((e) => e.kind)).toEqual(["single", "single"])
  })
})

describe("categoryOverrides", () => {
  it("applies a group's category to all its members and keeps the match word per chosen category", () => {
    const a = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high", matchWord: "aral" } })
    const b = txn({ counterpartyName: "Aral", suggestion: { categoryId: fuel, confidence: "high", matchWord: "aral" } })
    const single = txn({ counterpartyName: "Shell", suggestion: { categoryId: fuel, confidence: "high" } })
    const entries = groupReviewQueue([a, b, single], true)
    const key = entries[0].kind === "group" ? entries[0].key : ""

    const overrides = categoryOverrides(entries, new Map([[key, groceries]]))

    expect(batchReviewItems([a, b, single], true, () => true, overrides)).toEqual([
      { transactionId: a.id, categoryId: groceries, addMatchWords: ["aral"] },
      { transactionId: b.id, categoryId: groceries, addMatchWords: [] },
      { transactionId: single.id, categoryId: fuel, addMatchWords: [] },
    ])
  })
})

describe("batchReviewItems", () => {
  it("sends the proposed categories and an opted-in match word once per category", () => {
    const a = txn({ suggestion: { categoryId: fuel, confidence: "high", matchWord: "Aral" } })
    const b = txn({ suggestion: { categoryId: fuel, confidence: "high", matchWord: "aral" } })
    const c = txn({ suggestion: { categoryId: fuel, confidence: "high", matchWord: "shell" } })
    const d = txn({ categoryId: groceries, categorizationSource: "keyword" })
    const none = txn({})

    const items = batchReviewItems([a, b, c, d, none], true, (id) => id !== c.id)

    expect(items).toEqual([
      { transactionId: a.id, categoryId: fuel, addMatchWords: ["Aral"] },
      { transactionId: b.id, categoryId: fuel, addMatchWords: [] },
      { transactionId: c.id, categoryId: fuel, addMatchWords: [] },
      { transactionId: d.id, categoryId: groceries, addMatchWords: [] },
    ])
  })
})
