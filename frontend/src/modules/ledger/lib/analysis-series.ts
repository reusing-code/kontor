import type { TooltipRow } from "@/components/charts/chart-parts"
import { formatMinor } from "@/lib/format"
import type {
  LedgerCashFlowAnalysis,
  LedgerCategory,
  LedgerCategoryBreakdown,
  LedgerNetWorthAnalysis,
  LedgerSpendingAnalysis,
} from "@/modules/ledger/types"

export const categoryPaletteSize = 10
export const maxCategorySeries = 9
export const paletteColor = (index: number) => `var(--viz-cat-${(index % categoryPaletteSize) + 1})`
export const otherColor = "var(--viz-cat-other)"
export const uncategorizedColor = "var(--viz-cat-uncategorized)"
// tintSteps lighter (dark mode: darker) variants of every palette color for
// the subcategories of the two-ring chart, --viz-cat-N-tint-1..3.
export const tintSteps = 3
export const maxBreakdownChildren = 5
// tintColor: step 0 is the palette color itself.
export const tintColor = (index: number, step: number) =>
  step === 0 ? paletteColor(index) : `var(--viz-cat-${(index % categoryPaletteSize) + 1}-tint-${step})`

// parentsWithChildren lists the top-level categories that have subcategories,
// by name: the choices of the spending card's level select.
export function parentsWithChildren(categories: LedgerCategory[]): LedgerCategory[] {
  const ids = new Set(categories.map((c) => c.id))
  return categories
    .filter((c) => (!c.parentId || !ids.has(c.parentId)) && categories.some((child) => child.parentId === c.id))
    .sort((a, b) => a.name.localeCompare(b.name))
}

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
export function spendingTooltipRows(series: ChartSeries[], index: number, totalLabel: string): TooltipRow[] {
  const rows: TooltipRow[] = series
    .filter((s) => s.amountsMinor[index] !== 0)
    .map((s) => ({ name: s.label, value: formatMinor(s.amountsMinor[index]), color: s.color }))
  const total = series.reduce((sum, s) => sum + s.amountsMinor[index], 0)
  return [...rows, { name: totalLabel, value: formatMinor(total) }]
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

// netWorthSeries: the total, and with showAccounts one series per account.
// Accounts come in list order whatever the range, so their colors are stable.
export function netWorthSeries(analysis: LedgerNetWorthAnalysis, totalLabel: string, showAccounts: boolean): ChartSeries[] {
  const last = (values: number[]) => values[values.length - 1] ?? 0
  const total: ChartSeries = { key: "total", label: totalLabel, color: "var(--foreground)", amountsMinor: analysis.totalMinor, totalMinor: last(analysis.totalMinor) }
  if (!showAccounts) return [total]
  return [
    total,
    ...analysis.accounts.map((account, index) => ({
      key: account.accountId,
      label: account.name,
      color: paletteColor(index),
      amountsMinor: account.valuesMinor,
      totalMinor: last(account.valuesMinor),
    })),
  ]
}

// seriesTooltipRows lists every series of one axis position, zeros included.
export function seriesTooltipRows(series: ChartSeries[], index: number): TooltipRow[] {
  return series.map((s) => ({ name: s.label, value: formatMinor(s.amountsMinor[index]), color: s.color }))
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

export interface RingSlice {
  key: string
  label: string
  color: string
  amountMinor: number
  // the inner slice an outer slice lies in; for an inner slice its own key
  parentKey: string
  parentLabel: string
  // the top-level category of an inner slice (not for "other" and uncategorized)
  categoryId?: string
  // an outer arc left empty: drawn transparent, without tooltip
  placeholder?: boolean
}

export interface BreakdownRings {
  inner: RingSlice[]
  // in the order of inner, so every outer slice lies within its parent's arc
  outer: RingSlice[]
  totalMinor: number
}

// breakdownRings turns a breakdown response into the two rings. The inner ring
// is the top level like spendingChartSeries draws it (same colors, at most cap
// categories, the rest as "other", uncategorized last). Per inner slice the
// outer ring holds its largest childCap children and the rest as "other", in
// the parent's color and its tints; it stays empty for "other", uncategorized
// and a parent whose spending is booked only on itself.
export function breakdownRings(
  analysis: LedgerCategoryBreakdown,
  categories: LedgerCategory[],
  labels: { uncategorized: string; other: string; direct: (name: string) => string },
  cap: number = maxCategorySeries,
  childCap: number = maxBreakdownChildren,
): BreakdownRings {
  const colorIndex = siblingColorIndex(categories)
  const parents = analysis.categories.filter((c) => c.kind === "category")
  const drawn = parents.slice(0, cap)
  const slots = distinctPaletteIndexes(drawn.map((c, position) => colorIndex.get(c.categoryId ?? "") ?? position))
  const inner: RingSlice[] = []
  const outer: RingSlice[] = []
  const empty = (parent: RingSlice) =>
    outer.push({ ...parent, key: `${parent.key}:empty`, color: "transparent", placeholder: true, categoryId: undefined })

  drawn.forEach((c, position) => {
    const key = c.categoryId ?? `category-${position}`
    const parent: RingSlice = { key, label: c.name, color: paletteColor(slots[position]), amountMinor: c.amountMinor, parentKey: key, parentLabel: c.name, categoryId: c.categoryId }
    inner.push(parent)
    if (c.children.every((child) => child.kind === "direct")) {
      empty(parent)
      return
    }
    const rest = c.children.slice(childCap)
    const children = c.children.slice(0, childCap).map((child) => ({
      key: child.kind === "direct" ? `${key}:direct` : child.categoryId,
      label: child.kind === "direct" ? labels.direct(c.name) : child.name,
      amountMinor: child.amountMinor,
    }))
    if (rest.length > 0) {
      children.push({ key: `${key}:other`, label: labels.other, amountMinor: rest.reduce((sum, child) => sum + child.amountMinor, 0) })
    }
    children.forEach((child, index) =>
      outer.push({ ...child, color: tintColor(slots[position], index % (tintSteps + 1)), parentKey: key, parentLabel: c.name }),
    )
  })

  const rest = parents.slice(cap)
  if (rest.length > 0) {
    const other: RingSlice = { key: "other", label: labels.other, color: otherColor, amountMinor: rest.reduce((sum, c) => sum + c.amountMinor, 0), parentKey: "other", parentLabel: labels.other }
    inner.push(other)
    empty(other)
  }
  for (const c of analysis.categories) {
    if (c.kind === "category") continue
    const uncategorized: RingSlice = { key: c.kind, label: labels.uncategorized, color: uncategorizedColor, amountMinor: c.amountMinor, parentKey: c.kind, parentLabel: labels.uncategorized }
    inner.push(uncategorized)
    empty(uncategorized)
  }
  return { inner, outer, totalMinor: analysis.totalMinor }
}

// breakdownTooltipRows: the amount, the share of the total and, for an outer
// slice, the share of its parent (percentages in locale).
export function breakdownTooltipRows(
  slice: RingSlice,
  rings: BreakdownRings,
  labels: { amount: string; shareOfTotal: string; shareOfParent: (name: string) => string },
  locale: string,
): TooltipRow[] {
  const percent = new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 1 })
  const share = (whole: number) => percent.format(whole > 0 ? slice.amountMinor / whole : 0)
  const rows: TooltipRow[] = [
    { name: labels.amount, value: formatMinor(slice.amountMinor), color: slice.color },
    { name: labels.shareOfTotal, value: share(rings.totalMinor) },
  ]
  if (slice.parentKey !== slice.key) {
    const parent = rings.inner.find((s) => s.key === slice.parentKey)
    rows.push({ name: labels.shareOfParent(slice.parentLabel), value: share(parent?.amountMinor ?? 0) })
  }
  return rows
}
