import type { LedgerAnalysisRange } from "@/modules/ledger/types"

export type AnalysisPreset = "6m" | "12m" | "ytd" | "all"
export const analysisPresets: AnalysisPreset[] = ["6m", "12m", "ytd", "all"]

export type AnalysisRangeState =
  | { kind: "preset"; preset: AnalysisPreset }
  | { kind: "custom"; from: string; to: string }

export type AnalysisRangeProblem = "incomplete" | "fromAfterTo" | "toInFuture" | "tooLong"

// The backend refuses a range of more calendar months (maxLedgerAnalysisMonths).
export const maxAnalysisMonths = 1200

export const defaultAnalysisRange: AnalysisRangeState = { kind: "preset", preset: "12m" }

const pad = (n: number) => String(n).padStart(2, "0")

// Months are counted on the year and month numbers of the ISO string, so no
// timezone can move them.
const monthNumber = (isoDate: string) => Number(isoDate.slice(0, 4)) * 12 + Number(isoDate.slice(5, 7)) - 1

// The first day of the month `months` before the month of an ISO date.
function monthStart(isoDate: string, months: number): string {
  const index = monthNumber(isoDate) - months
  return `${Math.floor(index / 12)}-${pad((index % 12) + 1)}-01`
}

// presetRange: "N months" is the current month plus the N-1 months before it,
// up to today; "all" leaves both ends to the server.
export function presetRange(preset: AnalysisPreset, today: string): LedgerAnalysisRange {
  switch (preset) {
    case "6m":
      return { from: monthStart(today, 5), to: today }
    case "12m":
      return { from: monthStart(today, 11), to: today }
    case "ytd":
      return { from: `${today.slice(0, 4)}-01-01`, to: today }
    case "all":
      return {}
  }
}

function isISODate(value: string): boolean {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
}

export function customRangeProblem(from: string, to: string, today: string): AnalysisRangeProblem | undefined {
  if (!isISODate(from) || !isISODate(to)) return "incomplete"
  if (from > to) return "fromAfterTo"
  if (to > today) return "toInFuture"
  // also what a date input reports while a year is being typed ("0202-12-01")
  if (monthNumber(to) - monthNumber(from) + 1 > maxAnalysisMonths) return "tooLong"
  return undefined
}

export function resolveRange(
  state: AnalysisRangeState,
  today: string,
): { range?: LedgerAnalysisRange; problem?: AnalysisRangeProblem } {
  if (state.kind === "preset") return { range: presetRange(state.preset, today) }
  const problem = customRangeProblem(state.from, state.to, today)
  return problem ? { problem } : { range: { from: state.from, to: state.to } }
}

// analysisQuery builds the query string of an analysis request; empty values
// are left out.
export function analysisQuery(range: LedgerAnalysisRange, extra: Record<string, string | undefined> = {}): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries({ from: range.from, to: range.to, ...extra })) {
    if (value) params.set(key, value)
  }
  const query = params.toString()
  return query ? `?${query}` : ""
}

// formatMonth: "2026-03" -> "Mar 26".
export function formatMonth(month: string, locale: string): string {
  const date = new Date(Date.UTC(Number(month.slice(0, 4)), Number(month.slice(5, 7)) - 1, 1))
  return new Intl.DateTimeFormat(locale, { month: "short", year: "2-digit", timeZone: "UTC" }).format(date)
}
