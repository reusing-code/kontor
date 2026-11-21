import type { LedgerTransactionSplit } from "@/modules/ledger/types"

// What the split editor holds per part: the amount as typed (always
// positive, the transaction's sign is applied on save), category and note.
export interface SplitRow {
  amount: string
  categoryId: string
  note: string
}

export const maxSplitParts = 10
export const maxSplitNoteLength = 500

// parseAmountInput reads "150", "150,5" or "150.50" into positive minor units.
export function parseAmountInput(text: string): number | undefined {
  const match = /^(\d{1,9})(?:[.,](\d{1,2}))?$/.exec(text.trim())
  if (!match) return undefined
  const minor = Number(match[1]) * 100 + Number((match[2] ?? "").padEnd(2, "0"))
  return minor > 0 ? minor : undefined
}

export function formatAmountInput(minor: number, locale: string): string {
  return (Math.abs(minor) / 100).toLocaleString(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2, useGrouping: false })
}

export function rowsFromSplits(splits: LedgerTransactionSplit[] | undefined, locale: string): SplitRow[] {
  return (splits ?? []).map((part) => ({
    amount: formatAmountInput(part.amountMinor, locale),
    categoryId: part.categoryId ?? "",
    note: part.note ?? "",
  }))
}

export type SplitProblem = "amount" | "category" | "total" | "tooMany" | "note"

// splitProblem mirrors the backend rules: every part has a positive amount
// and a category, and together the parts stay below the transaction amount.
export function splitProblem(rows: SplitRow[], amountMinor: number): SplitProblem | null {
  if (rows.length > maxSplitParts) return "tooMany"
  if (rows.some((row) => parseAmountInput(row.amount) === undefined)) return "amount"
  if (rows.some((row) => !row.categoryId)) return "category"
  if (rows.some((row) => row.note.trim().length > maxSplitNoteLength)) return "note"
  if (rows.length > 0 && splitTotal(rows) >= Math.abs(amountMinor)) return "total"
  return null
}

function splitTotal(rows: SplitRow[]): number {
  return rows.reduce((sum, row) => sum + (parseAmountInput(row.amount) ?? 0), 0)
}

// splitRemainder is what stays with the transaction's own category, signed
// like the transaction; amounts that do not parse count as zero.
export function splitRemainder(rows: SplitRow[], amountMinor: number): number {
  return Math.sign(amountMinor) * (Math.abs(amountMinor) - splitTotal(rows))
}

export function splitsFromRows(rows: SplitRow[], amountMinor: number): LedgerTransactionSplit[] {
  const sign = amountMinor < 0 ? -1 : 1
  return rows.map((row) => ({
    amountMinor: sign * (parseAmountInput(row.amount) ?? 0),
    categoryId: row.categoryId,
    ...(row.note.trim() ? { note: row.note.trim() } : {}),
  }))
}
