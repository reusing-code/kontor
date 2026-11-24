import { useTranslation } from "react-i18next"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { useLedgerCategoryBreakdown } from "@/modules/ledger/hooks/use-ledger-analysis"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { breakdownRings, parentsWithChildren, type RingSlice } from "@/modules/ledger/lib/analysis-series"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"
import { CategoryBreakdownChart } from "@/modules/ledger/components/ledger-analysis-charts"
import { ChartLegend } from "@/components/charts/chart-parts"

// LedgerAnalysisBreakdown shows the spending of the range as two rings: the
// top-level categories inside, their subcategories outside. Selecting a
// top-level category that has subcategories (slice or legend entry) calls
// onSelectCategory.
export function LedgerAnalysisBreakdown({ range, onSelectCategory }: { range: LedgerAnalysisRange; onSelectCategory: (categoryId: string) => void }) {
  const { t } = useTranslation()
  const { data: categories = [] } = useLedgerCategories()
  const query = useLedgerCategoryBreakdown(range)

  const rings = query.data
    ? breakdownRings(query.data, categories, {
        uncategorized: t("ledger.analysis.breakdown.uncategorized"),
        other: t("ledger.analysis.breakdown.other"),
        direct: (name) => t("ledger.analysis.breakdown.direct", { name }),
      })
    : undefined
  const selectable = new Set(parentsWithChildren(categories).map((c) => c.id))
  const canSelect = (slice: RingSlice) => slice.categoryId !== undefined && selectable.has(slice.categoryId)
  const select = (slice: RingSlice) => {
    if (slice.categoryId) onSelectCategory(slice.categoryId)
  }
  const dropped = query.data?.droppedInflows ?? 0

  return (
    <LedgerAnalysisCard
      title={t("ledger.analysis.breakdown.title")}
      isPending={query.isPending}
      isError={query.isError}
      isRefreshing={query.isPlaceholderData}
      onRetry={() => void query.refetch()}
      isEmpty={!rings || rings.inner.length === 0}
      emptyText={t("ledger.analysis.breakdown.empty")}
      footer={
        dropped > 0 && (
          <p className="mt-3 text-xs text-muted-foreground">{t("ledger.analysis.breakdown.droppedInflows", { count: dropped })}</p>
        )
      }
    >
      {rings && (
        <>
          <CategoryBreakdownChart rings={rings} canSelect={canSelect} onSelect={select} />
          <ChartLegend
            items={rings.inner.map((slice) => ({
              label: slice.label,
              color: slice.color,
              onSelect: canSelect(slice) ? () => select(slice) : undefined,
            }))}
          />
          {rings.inner.some(canSelect) && <p className="mt-2 text-center text-xs text-muted-foreground">{t("ledger.analysis.breakdown.selectHint")}</p>}
        </>
      )}
    </LedgerAnalysisCard>
  )
}
