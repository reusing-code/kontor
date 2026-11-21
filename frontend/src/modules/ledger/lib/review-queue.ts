import type { LedgerBatchReviewItem, LedgerReviewQueueItem, LedgerSuggestionConfidence } from "@/modules/ledger/types"

// A category the queue proposes for a transaction: one a match word already
// assigned (still needs review), or an LLM suggestion for an uncategorized one.
export interface ReviewProposal {
  categoryId: string
  kind: "assigned" | "suggested"
  confidence?: LedgerSuggestionConfidence
  matchWord?: string
}

export function reviewProposal(item: LedgerReviewQueueItem, llm: boolean): ReviewProposal | undefined {
  if (item.transferPairTransactionId) return undefined
  if (item.categoryId) return { categoryId: item.categoryId, kind: "assigned" }
  if (llm && item.suggestion) {
    return {
      categoryId: item.suggestion.categoryId,
      kind: "suggested",
      confidence: item.suggestion.confidence,
      matchWord: item.suggestion.matchWord,
    }
  }
  return undefined
}

const stopWords = new Set([
  "gmbh", "und", "the", "von", "der", "die", "das", "bei", "for", "and", "inc", "ltd", "llc",
  "sagt", "danke", "dankt", "www", "com", "sepa", "online",
])

// counterpartyKey normalizes a counterparty like the backend's recurring
// detection: lower-case words of at least 3 letters or digits, without stop
// words, pure numbers (store or terminal IDs) and repeats.
export function counterpartyKey(name: string | undefined): string {
  const words = (name ?? "").toLowerCase().split(/[^\p{L}\p{N}]+/u)
  // code points, like the backend's rune count
  const kept = words.filter((w, i) => Array.from(w).length >= 3 && !stopWords.has(w) && !/^\p{N}+$/u.test(w) && words.indexOf(w) === i)
  return kept.join(" ")
}

export type ReviewEntry =
  | { kind: "single"; item: LedgerReviewQueueItem }
  | { kind: "group"; key: string; items: LedgerReviewQueueItem[]; proposal: ReviewProposal }

// groupReviewQueue keeps the queue order and folds transactions with the same
// counterparty and the same proposal into one group, placed where its first
// (newest) member is. Transactions without a proposal or counterparty stay
// single, and so does a group of one.
export function groupReviewQueue(items: LedgerReviewQueueItem[], llm: boolean): ReviewEntry[] {
  const groups = new Map<string, LedgerReviewQueueItem[]>()
  const keyOf = new Map<string, string>()
  for (const item of items) {
    const proposal = reviewProposal(item, llm)
    const counterparty = counterpartyKey(item.counterpartyName)
    if (!proposal || !counterparty) continue
    const key = `${counterparty}|${proposal.kind}|${proposal.categoryId}`
    keyOf.set(item.id, key)
    groups.set(key, [...(groups.get(key) ?? []), item])
  }

  const placed = new Set<string>()
  return items.flatMap((item): ReviewEntry[] => {
    const key = keyOf.get(item.id)
    const members = key ? groups.get(key) ?? [] : []
    if (!key || members.length < 2) return [{ kind: "single", item }]
    if (placed.has(key)) return []
    placed.add(key)
    return [{ kind: "group", key, items: members, proposal: groupProposal(members, llm) }]
  })
}

const confidenceRank: Record<LedgerSuggestionConfidence, number> = { low: 0, medium: 1, high: 2 }

// The group shows its weakest confidence and the match word most members
// were proposed with.
function groupProposal(items: LedgerReviewQueueItem[], llm: boolean): ReviewProposal {
  const proposals = items.flatMap((item) => reviewProposal(item, llm) ?? [])
  const confidences = proposals.flatMap((p) => p.confidence ?? [])
  const words = proposals.flatMap((p) => p.matchWord ?? [])
  const counts = new Map<string, number>()
  words.forEach((w) => counts.set(w, (counts.get(w) ?? 0) + 1))
  const [matchWord] = [...counts.entries()].sort((a, b) => b[1] - a[1]).map(([w]) => w)
  const [confidence] = [...confidences].sort((a, b) => confidenceRank[a] - confidenceRank[b])
  return { ...proposals[0], confidence, matchWord }
}

export function entryItems(entry: ReviewEntry): LedgerReviewQueueItem[] {
  return entry.kind === "group" ? entry.items : [entry.item]
}

// batchReviewItems turns the selected transactions into review-batch items.
// A match word goes along only when it was opted in, and only once per
// category and word.
export function batchReviewItems(
  items: LedgerReviewQueueItem[],
  llm: boolean,
  matchWordOn: (transactionId: string) => boolean,
): LedgerBatchReviewItem[] {
  const sentWords = new Set<string>()
  return items.flatMap((item) => {
    const proposal = reviewProposal(item, llm)
    if (!proposal) return []
    const word = proposal.matchWord && matchWordOn(item.id) ? proposal.matchWord : undefined
    const wordKey = word && `${proposal.categoryId}|${word.toLowerCase()}`
    const addMatchWords = wordKey && !sentWords.has(wordKey) ? [word] : []
    if (wordKey) sentWords.add(wordKey)
    return [{ transactionId: item.id, categoryId: proposal.categoryId, addMatchWords }]
  })
}
