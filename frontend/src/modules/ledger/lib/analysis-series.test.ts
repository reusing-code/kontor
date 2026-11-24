import { describe, expect, it } from "vitest"
import type { LedgerBreakdownCategory, LedgerCategory, LedgerCategoryBreakdown, LedgerSpendingAnalysis, LedgerSpendingSeries } from "@/modules/ledger/types"
import {
  breakdownRings,
  breakdownTooltipRows,
  cashFlowSeries,
  chartRows,
  formatAmountCompact,
  netWorthSeries,
  otherColor,
  paletteColor,
  parentsWithChildren,
  seriesTooltipRows,
  siblingColorIndex,
  spendingChartSeries,
  spendingTooltipRows,
  tintColor,
  uncategorizedColor,
} from "@/modules/ledger/lib/analysis-series"

function category(id: string, createdAt: string, parentId?: string): LedgerCategory {
  return { id, name: id.toUpperCase(), parentId, matchWords: [], createdAt, updatedAt: createdAt }
}

const categories = [
  category("food", "2026-01-01T10:00:00Z"),
  category("car", "2026-01-02T10:00:00Z"),
  category("home", "2026-01-03T10:00:00Z"),
  category("groceries", "2026-02-02T10:00:00Z", "food"),
  category("restaurants", "2026-02-01T10:00:00Z", "food"),
  category("orphan", "2026-01-04T10:00:00Z", "deleted-parent"),
]

function series(categoryId: string, amountsMinor: number[]): LedgerSpendingSeries {
  return { kind: "category", categoryId, name: categoryId.toUpperCase(), amountsMinor, totalMinor: amountsMinor.reduce((a, b) => a + b, 0) }
}

function analysis(list: LedgerSpendingSeries[], parentCategoryId?: string): LedgerSpendingAnalysis {
  return { from: "2026-01-01", to: "2026-02-28", months: ["2026-01", "2026-02"], parentCategoryId, series: list, totalsMinor: [0, 0], excludedCurrencies: [] }
}

const labels = { uncategorized: "Uncategorized", other: "Other", direct: "Directly" }

describe("siblingColorIndex", () => {
  it("orders the top level by creation, a missing parent counts as top level", () => {
    expect([...siblingColorIndex(categories)]).toEqual([["food", 0], ["car", 1], ["home", 2], ["orphan", 3]])
  })

  it("does not depend on the input order", () => {
    expect(siblingColorIndex([...categories].reverse())).toEqual(siblingColorIndex(categories))
  })

  it("orders the children of a parent", () => {
    expect([...siblingColorIndex(categories, "food")]).toEqual([["restaurants", 0], ["groceries", 1]])
  })

  it("breaks a creation tie by id", () => {
    const same = [category("b", "2026-01-01T10:00:00Z"), category("a", "2026-01-01T10:00:00Z")]
    expect([...siblingColorIndex(same)]).toEqual([["a", 0], ["b", 1]])
  })

  it("cycles the palette beyond ten", () => {
    expect(paletteColor(0)).toBe("var(--viz-cat-1)")
    expect(paletteColor(9)).toBe("var(--viz-cat-10)")
    expect(paletteColor(10)).toBe("var(--viz-cat-1)")
  })
})

describe("spendingChartSeries", () => {
  it("keeps a category's color whatever the response order", () => {
    const first = spendingChartSeries(analysis([series("home", [5, 5]), series("food", [1, 1])]), categories, labels)
    const second = spendingChartSeries(analysis([series("food", [9, 9]), series("car", [2, 2]), series("home", [1, 1])]), categories, labels)
    const color = (list: typeof first, key: string) => list.find((s) => s.key === key)?.color
    expect(first.map((s) => s.key)).toEqual(["home", "food"])
    expect(color(first, "food")).toBe("var(--viz-cat-1)")
    expect(color(second, "food")).toBe("var(--viz-cat-1)")
    expect(color(first, "home")).toBe("var(--viz-cat-3)")
    expect(color(second, "home")).toBe("var(--viz-cat-3)")
    expect(color(second, "car")).toBe("var(--viz-cat-2)")
  })

  it("gives the drawn series distinct colors with more than ten siblings", () => {
    const many = Array.from({ length: 23 }, (_, i) => category(`c${String(i).padStart(2, "0")}`, `2026-01-${String(i + 1).padStart(2, "0")}T10:00:00Z`))
    // c00/c10/c20 share index 0, c01/c11/c21 index 1, c02/c12/c22 index 2
    const drawn = ["c10", "c00", "c20", "c01", "c11", "c05", "c22", "c12", "c02"]
    const got = spendingChartSeries(analysis(drawn.map((id) => series(id, [1, 1]))), many, labels)

    expect(got.map((s) => s.key)).toEqual(drawn)
    expect(new Set(got.map((s) => s.color)).size).toBe(9)
    const color = Object.fromEntries(got.map((s) => [s.key, s.color]))
    // the first drawn of each group and the series without a collision keep their index
    expect([color.c10, color.c01, color.c22, color.c05]).toEqual([1, 2, 3, 6].map((n) => `var(--viz-cat-${n})`))
    // the others move, in drawn order, to the colors no drawn series prefers
    expect([color.c00, color.c20, color.c11, color.c12, color.c02]).toEqual([4, 5, 7, 8, 9].map((n) => `var(--viz-cat-${n})`))

    // the same holds for every selection of nine out of the 23
    for (let offset = 0; offset < many.length; offset++) {
      const ids = Array.from({ length: 9 }, (_, i) => many[(offset + i * 5) % many.length].id)
      const colors = spendingChartSeries(analysis(ids.map((id) => series(id, [1, 1]))), many, labels).map((s) => s.color)
      expect(new Set(colors).size).toBe(9)
    }
  })

  it("moves only the colliding category when one collision is drawn", () => {
    const eleven = Array.from({ length: 11 }, (_, i) => category(`c${String(i).padStart(2, "0")}`, `2026-01-${String(i + 1).padStart(2, "0")}T10:00:00Z`))
    const colors = (drawn: string[]) => {
      const got = spendingChartSeries(analysis(drawn.map((id) => series(id, [1, 1]))), eleven, labels)
      return Object.fromEntries(got.map((s) => [s.key, s.color]))
    }
    // only c00 and c10 prefer the same color
    const withCollision = colors(["c00", "c10", "c01", "c02", "c03"])
    const without = colors(["c00", "c01", "c02", "c03"])

    for (const [id, n] of [["c00", 1], ["c01", 2], ["c02", 3], ["c03", 4]] as const) {
      expect(withCollision[id], id).toBe(`var(--viz-cat-${n})`)
      expect(without[id], id).toBe(`var(--viz-cat-${n})`)
    }
    expect(withCollision.c10).toBe("var(--viz-cat-5)")
    // the larger one keeps the color whichever of the two it is
    expect(colors(["c10", "c00", "c01"])).toEqual({ c10: "var(--viz-cat-1)", c00: "var(--viz-cat-3)", c01: "var(--viz-cat-2)" })
  })

  it("keeps every stable index when the drawn series do not collide", () => {
    const many = Array.from({ length: 23 }, (_, i) => category(`c${String(i).padStart(2, "0")}`, `2026-01-${String(i + 1).padStart(2, "0")}T10:00:00Z`))
    const drawn = ["c19", "c03", "c12", "c20", "c07", "c15", "c04", "c21", "c08"]
    const got = spendingChartSeries(analysis(drawn.map((id) => series(id, [1, 1]))), many, labels)
    expect(got.map((s) => s.color)).toEqual([10, 4, 3, 1, 8, 6, 5, 2, 9].map((n) => `var(--viz-cat-${n})`))
  })

  it("gives colliding subcategories distinct colors too", () => {
    const children = Array.from({ length: 12 }, (_, i) => category(`s${String(i).padStart(2, "0")}`, `2026-02-${String(i + 1).padStart(2, "0")}T10:00:00Z`, "home"))
    const got = spendingChartSeries(analysis([series("s11", [1, 1]), series("s01", [1, 1]), series("s10", [1, 1]), series("s00", [1, 1])], "home"), [...categories, ...children], labels)
    expect(got.map((s) => s.color)).toEqual([2, 3, 1, 4].map((n) => `var(--viz-cat-${n})`))
  })

  it("sums the series beyond the cap into Other and puts the neutral series last", () => {
    const got = spendingChartSeries(
      analysis([
        series("food", [100, 200]),
        { kind: "uncategorized", name: "", amountsMinor: [70, 0], totalMinor: 70 },
        series("car", [50, 60]),
        series("home", [5, 6]),
        series("orphan", [1, 2]),
      ]),
      categories,
      labels,
      2,
    )
    expect(got.map((s) => [s.key, s.label, s.color, s.amountsMinor, s.totalMinor])).toEqual([
      ["food", "FOOD", "var(--viz-cat-1)", [100, 200], 300],
      ["car", "CAR", "var(--viz-cat-2)", [50, 60], 110],
      ["other", "Other", otherColor, [6, 8], 14],
      ["uncategorized", "Uncategorized", uncategorizedColor, [70, 0], 70],
    ])
  })

  it("has no Other at the cap", () => {
    const got = spendingChartSeries(analysis([series("food", [1, 1]), series("car", [1, 1])]), categories, labels, 2)
    expect(got.map((s) => s.key)).toEqual(["food", "car"])
  })

  it("colors subcategories among their siblings and labels the direct series", () => {
    const got = spendingChartSeries(
      analysis([series("groceries", [300, 0]), series("restaurants", [-80, 300]), { kind: "direct", categoryId: "food", name: "", amountsMinor: [50, 0], totalMinor: 50 }], "food"),
      categories,
      labels,
    )
    expect(got.map((s) => [s.key, s.label, s.color])).toEqual([
      ["groceries", "GROCERIES", "var(--viz-cat-2)"],
      ["restaurants", "RESTAURANTS", "var(--viz-cat-1)"],
      ["direct", "Directly", uncategorizedColor],
    ])
  })

  it("falls back to the position for a category it does not know", () => {
    const got = spendingChartSeries(analysis([series("food", [1, 1]), series("new", [1, 1])]), categories, labels)
    expect(got[1].color).toBe("var(--viz-cat-2)")
  })
})

describe("chart rows and tooltip", () => {
  const chart = spendingChartSeries(
    analysis([series("food", [12345, 0]), series("car", [-500, 700]), { kind: "uncategorized", name: "", amountsMinor: [0, 0], totalMinor: 0 }]),
    categories,
    labels,
  )

  it("builds one row per month", () => {
    expect(chartRows(["2026-01", "2026-02"], "month", chart)).toEqual([
      { month: "2026-01", food: 12345, car: -500, uncategorized: 0 },
      { month: "2026-02", food: 0, car: 700, uncategorized: 0 },
    ])
  })

  it("lists the non-zero series of a month and the total", () => {
    expect(spendingTooltipRows(chart, 0, "Total")).toEqual([
      { name: "FOOD", value: "€123.45", color: "var(--viz-cat-1)" },
      { name: "CAR", value: "-€5.00", color: "var(--viz-cat-2)" },
      { name: "Total", value: "€118.45" },
    ])
    expect(spendingTooltipRows(chart, 1, "Total")).toEqual([
      { name: "CAR", value: "€7.00", color: "var(--viz-cat-2)" },
      { name: "Total", value: "€7.00" },
    ])
  })

  it("shortens axis amounts", () => {
    expect(formatAmountCompact(0, "en")).toBe("€0")
    expect(formatAmountCompact(50000, "en")).toBe("€500")
    expect(formatAmountCompact(123456, "en")).toBe("€1.2K")
    expect(formatAmountCompact(-250000000, "en")).toBe("-€2.5M")
  })
})

describe("cashFlowSeries", () => {
  const flow = cashFlowSeries(
    { from: "2026-01-01", to: "2026-02-28", months: ["2026-01", "2026-02"], incomeMinor: [300000, 0], expensesMinor: [120050, 4000], netMinor: [179950, -4000], excludedCurrencies: [] },
    { income: "Income", expenses: "Expenses", net: "Net result" },
  )

  it("has income, expenses and the net result with their colors", () => {
    expect(flow.map((s) => [s.key, s.label, s.color, s.totalMinor])).toEqual([
      ["income", "Income", "var(--viz-income)", 300000],
      ["expenses", "Expenses", "var(--viz-expenses)", 124050],
      ["net", "Net result", "var(--viz-net)", 175950],
    ])
  })

  it("lists all three in the tooltip, zeros included", () => {
    expect(seriesTooltipRows(flow, 1)).toEqual([
      { name: "Income", value: "€0.00", color: "var(--viz-income)" },
      { name: "Expenses", value: "€40.00", color: "var(--viz-expenses)" },
      { name: "Net result", value: "-€40.00", color: "var(--viz-net)" },
    ])
  })
})

describe("netWorthSeries", () => {
  const worth = {
    from: "2026-09-01",
    to: "2026-10-31",
    dates: ["2026-09-30", "2026-10-31"],
    totalMinor: [1200000, 1250000],
    accounts: [
      { accountId: "acc-giro", name: "Giro", valuesMinor: [200000, 250000] },
      { accountId: "acc-savings", name: "Savings", valuesMinor: [1000000, 1000000] },
    ],
    excludedAccounts: [],
  }

  it("is the total alone by default", () => {
    expect(netWorthSeries(worth, "Total", false)).toEqual([
      { key: "total", label: "Total", color: "var(--foreground)", amountsMinor: [1200000, 1250000], totalMinor: 1250000 },
    ])
  })

  it("adds the accounts in response order with palette colors", () => {
    const series = netWorthSeries(worth, "Total", true)
    expect(series.map((s) => [s.key, s.label, s.color, s.totalMinor])).toEqual([
      ["total", "Total", "var(--foreground)", 1250000],
      ["acc-giro", "Giro", "var(--viz-cat-1)", 250000],
      ["acc-savings", "Savings", "var(--viz-cat-2)", 1000000],
    ])
    expect(seriesTooltipRows(series, 0)).toEqual([
      { name: "Total", value: "€12,000.00", color: "var(--foreground)" },
      { name: "Giro", value: "€2,000.00", color: "var(--viz-cat-1)" },
      { name: "Savings", value: "€10,000.00", color: "var(--viz-cat-2)" },
    ])
  })

  it("has no values without dates", () => {
    expect(netWorthSeries({ ...worth, dates: [], totalMinor: [], accounts: [] }, "Total", true)).toEqual([
      { key: "total", label: "Total", color: "var(--foreground)", amountsMinor: [], totalMinor: 0 },
    ])
  })
})

describe("parentsWithChildren", () => {
  it("lists the top-level categories with subcategories by name", () => {
    const tree = [...categories, category("car-parts", "2026-03-01T10:00:00Z", "car")]
    expect(parentsWithChildren(tree).map((c) => c.id)).toEqual(["car", "food"])
  })
})

describe("breakdownRings", () => {
  const ringLabels = { uncategorized: "Uncategorized", other: "Other", direct: (name: string) => `${name} (directly)` }

  function parent(categoryId: string, children: Array<[string, number]>): LedgerBreakdownCategory {
    const list = children.map(([id, amountMinor]) =>
      id === "direct"
        ? { kind: "direct" as const, categoryId, name: "", amountMinor }
        : { kind: "category" as const, categoryId: id, name: id.toUpperCase(), amountMinor },
    )
    return { kind: "category", categoryId, name: categoryId.toUpperCase(), amountMinor: list.reduce((sum, c) => sum + c.amountMinor, 0), children: list }
  }

  function breakdown(list: LedgerBreakdownCategory[]): LedgerCategoryBreakdown {
    return { from: "2026-01-01", to: "2026-02-28", totalMinor: list.reduce((sum, c) => sum + c.amountMinor, 0), categories: list, droppedInflows: 0, excludedCurrencies: [] }
  }

  const uncategorized: LedgerBreakdownCategory = { kind: "uncategorized", name: "", amountMinor: 50, children: [] }

  it("lines the children up with their parents and sums them to the parent", () => {
    const rings = breakdownRings(breakdown([parent("food", [["groceries", 300], ["direct", 100]]), parent("home", [["direct", 200]]), uncategorized]), categories, ringLabels)
    expect(rings.inner.map((s) => [s.key, s.label, s.amountMinor])).toEqual([["food", "FOOD", 400], ["home", "HOME", 200], ["uncategorized", "Uncategorized", 50]])
    expect(rings.outer.map((s) => [s.key, s.label, s.amountMinor, s.parentKey, !!s.placeholder])).toEqual([
      ["groceries", "GROCERIES", 300, "food", false],
      ["food:direct", "FOOD (directly)", 100, "food", false],
      // only booked on itself, and uncategorized: empty outer arcs
      ["home:empty", "HOME", 200, "home", true],
      ["uncategorized:empty", "Uncategorized", 50, "uncategorized", true],
    ])
    for (const inner of rings.inner) {
      expect(rings.outer.filter((s) => s.parentKey === inner.key).reduce((sum, s) => sum + s.amountMinor, 0)).toBe(inner.amountMinor)
    }
    expect(rings.outer.filter((s) => s.placeholder).every((s) => s.color === "transparent" && s.categoryId === undefined)).toBe(true)
    expect(rings.totalMinor).toBe(650)
  })

  it("uses the bar chart's colors inside and the parent's tints outside", () => {
    const response = breakdown([parent("home", [["a", 50], ["b", 40], ["c", 30], ["d", 20], ["e", 10]]), parent("food", [["groceries", 30]])])
    const rings = breakdownRings(response, categories, ringLabels)
    const bars = spendingChartSeries(
      analysis([series("home", [0, 150]), series("food", [0, 30])]),
      categories,
      labels,
    )
    expect(rings.inner.map((s) => s.color)).toEqual(bars.map((s) => s.color))
    expect(rings.outer.map((s) => s.color)).toEqual([
      "var(--viz-cat-3)",
      "var(--viz-cat-3-tint-1)",
      "var(--viz-cat-3-tint-2)",
      "var(--viz-cat-3-tint-3)",
      "var(--viz-cat-3)",
      "var(--viz-cat-1)",
    ])
    expect(tintColor(10, 2)).toBe("var(--viz-cat-1-tint-2)")
  })

  it("sums the children beyond the cap and the top levels beyond the cap into Other", () => {
    const many = Array.from({ length: 11 }, (_, i) => category(`t${String(i).padStart(2, "0")}`, `2026-01-${String(i + 1).padStart(2, "0")}T10:00:00Z`))
    const tops = many.map((c, i) => parent(c.id, i === 0 ? [["a", 60], ["b", 50], ["c", 40], ["d", 30], ["e", 20], ["f", 10], ["g", 5]] : [["direct", 100 - i]]))
    const rings = breakdownRings(breakdown([...tops, uncategorized]), many, ringLabels)
    expect(rings.inner.map((s) => s.key)).toEqual([...many.slice(0, 9).map((c) => c.id), "other", "uncategorized"])
    const other = rings.inner[9]
    expect([other.label, other.amountMinor, other.color, other.categoryId]).toEqual(["Other", 91 + 90, otherColor, undefined])
    expect(rings.inner[10].color).toBe(uncategorizedColor)
    const first = rings.outer.filter((s) => s.parentKey === "t00")
    expect(first.map((s) => [s.label, s.amountMinor])).toEqual([["A", 60], ["B", 50], ["C", 40], ["D", 30], ["E", 20], ["Other", 15]])
    expect(rings.outer.find((s) => s.parentKey === "other")?.placeholder).toBe(true)
  })

  it("lists the amount, the share of the total and of the parent", () => {
    const rings = breakdownRings(breakdown([parent("food", [["groceries", 300], ["direct", 100]]), uncategorized]), categories, ringLabels)
    const tooltipLabels = { amount: "Amount", shareOfTotal: "Of all", shareOfParent: (name: string) => `Of ${name}` }
    expect(breakdownTooltipRows(rings.inner[0], rings, tooltipLabels, "en-US").map((r) => [r.name, r.value])).toEqual([["Amount", "€4.00"], ["Of all", "88.9%"]])
    expect(breakdownTooltipRows(rings.outer[0], rings, tooltipLabels, "en-US").map((r) => [r.name, r.value])).toEqual([
      ["Amount", "€3.00"],
      ["Of all", "66.7%"],
      ["Of FOOD", "75%"],
    ])
  })
})
