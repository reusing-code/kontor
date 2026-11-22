import type { TooltipRow } from "@/components/charts/chart-parts"
import type { LedgerCashFlowAnalysis, LedgerCategory, LedgerSpendingAnalysis } from "@/modules/ledger/types"
import { formatAmountMinor } from "@/modules/ledger/lib/ledger-utils"

export const categoryPaletteSize = 10
export const maxCategorySeries = 9
export const paletteColor = (index: number) => `var(--viz-cat-${(index % categoryPaletteSize) + 1})`
export const otherColor = "var(--viz-cat-other)"
export const uncategorizedColor = "var(--viz-cat-uncategorized)"

// siblingColorIndex gives every category below parentId (the top level without
// it) its position among its siblings by createdAt, then id. It depends only on
// the category tree, so it is the same whatever the response holds; it is the
// preferred color of a category (see distinctPaletteIndexes).
export function siblingColorIndex(categories: LedgerCategory[], parentId?: string): Map<string, number> {
  const ids = new Set(categories.map((c) => c.id))
  const siblings = categories
    .filter((c) => (parentId ? c.parentId === parentId : !c.parentId || !ids.has(c.parentId)))
    .sort((a, b) => a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id))
  return new Map(siblings.map((c, index) => [c.id, index]))
}

// distinctPaletteIndexes gives the series drawn in one chart different palette
// colors, in two passes over the drawn order. First every series whose
// preferred index is still free takes it, so of the series that prefer the same
// color the first (largest) one keeps it and a series that collides with none
// always keeps its own. Then each remaining series takes the next free index
// above its preferred one, which no drawn series prefers. Beyond the palette
// size colors repeat.
export function distinctPaletteIndexes(preferred: number[]): number[] {
  const wanted = preferred.map((index) => index % categoryPaletteSize)
  const taken = new Set<number>()
  const slots: Array<number | undefined> = wanted.map((slot) => {
    if (taken.has(slot)) return undefined
    taken.add(slot)
    return slot
  })
  return slots.map((kept, position) => {
    if (kept !== undefined) return kept
    let slot = wanted[position]
    for (let step = 0; step < categoryPaletteSize && taken.has(slot); step++) {
      slot = (slot + 1) % categoryPaletteSize
    }
    taken.add(slot)
    return slot
  })
}

export interface ChartSeries {
  key: string
  label: string
  color: string
  amountsMinor: number[]
  totalMinor: number
}

// spendingChartSeries turns a spending response into what the chart draws:
// the category series in response order up to cap (each in its own color),
// the rest summed into "other", then the series without a category of their
// own in the neutral color.
export function spendingChartSeries(
  analysis: LedgerSpendingAnalysis,
  categories: LedgerCategory[],
  labels: { uncategorized: string; other: string; direct: string },
  cap: number = maxCategorySeries,
): ChartSeries[] {
  const colorIndex = siblingColorIndex(categories, analysis.parentCategoryId)
  const categorySeries = analysis.series.filter((s) => s.kind === "category")
  const drawn = categorySeries.slice(0, cap)
  // Siblings beyond the palette size share an index; the larger series (drawn
  // first) keeps its color and the later one moves to a color nobody prefers.
  const slots = distinctPaletteIndexes(drawn.map((s, position) => colorIndex.get(s.categoryId ?? "") ?? position))
  const shown: ChartSeries[] = drawn.map((s, position) => ({
    key: s.categoryId ?? `category-${position}`,
    label: s.name,
    color: paletteColor(slots[position]),
    amountsMinor: s.amountsMinor,
    totalMinor: s.totalMinor,
  }))
  const rest = categorySeries.slice(cap)
  if (rest.length > 0) {
    shown.push({
      key: "other",
      label: labels.other,
      color: otherColor,
      amountsMinor: analysis.months.map((_, month) => rest.reduce((sum, s) => sum + s.amountsMinor[month], 0)),
      totalMinor: rest.reduce((sum, s) => sum + s.totalMinor, 0),
    })
  }
  for (const s of analysis.series) {
    if (s.kind === "category") continue
    shown.push({
      key: s.kind,
      label: s.kind === "direct" ? labels.direct : labels.uncategorized,
      color: uncategorizedColor,
      amountsMinor: s.amountsMinor,
      totalMinor: s.totalMinor,
    })
  }
  return shown
}

// chartRows turns the columns into one row per axis value, keyed by series.
export function chartRows(axis: string[], axisKey: string, series: ChartSeries[]): Array<Record<string, string | number>> {
  return axis.map((value, index) => {
    const row: Record<string, string | number> = { [axisKey]: value }
    for (const s of series) row[s.key] = s.amountsMinor[index]
    return row
  })
}

// spendingTooltipRows lists the non-zero series of one month and their sum.
export function spendingTooltipRows(series: ChartSeries[], index: number, totalLabel: string, locale: string): TooltipRow[] {
  const rows: TooltipRow[] = series
    .filter((s) => s.amountsMinor[index] !== 0)
    .map((s) => ({ name: s.label, value: formatAmountMinor(s.amountsMinor[index], "EUR", locale), color: s.color }))
  const total = series.reduce((sum, s) => sum + s.amountsMinor[index], 0)
  return [...rows, { name: totalLabel, value: formatAmountMinor(total, "EUR", locale) }]
}

// cashFlowSeries: income and expenses (both positive, drawn as bars) and the
// net result (drawn as a line).
export function cashFlowSeries(analysis: LedgerCashFlowAnalysis, labels: { income: string; expenses: string; net: string }): ChartSeries[] {
  const sum = (values: number[]) => values.reduce((total, value) => total + value, 0)
  return [
    { key: "income", label: labels.income, color: "var(--viz-income)", amountsMinor: analysis.incomeMinor, totalMinor: sum(analysis.incomeMinor) },
    { key: "expenses", label: labels.expenses, color: "var(--viz-expenses)", amountsMinor: analysis.expensesMinor, totalMinor: sum(analysis.expensesMinor) },
    { key: "net", label: labels.net, color: "var(--viz-net)", amountsMinor: analysis.netMinor, totalMinor: sum(analysis.netMinor) },
  ]
}

// seriesTooltipRows lists every series of one axis position, zeros included.
export function seriesTooltipRows(series: ChartSeries[], index: number, locale: string): TooltipRow[] {
  return series.map((s) => ({ name: s.label, value: formatAmountMinor(s.amountsMinor[index], "EUR", locale), color: s.color }))
}

// formatAmountCompact: 123456 minor units -> "€1.2K" (axis ticks).
export function formatAmountCompact(amountMinor: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency: "EUR",
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(amountMinor / 100)
}
