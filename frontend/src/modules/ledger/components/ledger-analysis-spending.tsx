import { useTranslation } from "react-i18next"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { useLedgerSpendingAnalysis } from "@/modules/ledger/hooks/use-ledger-analysis"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { parentsWithChildren, spendingChartSeries } from "@/modules/ledger/lib/analysis-series"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"
import { SpendingChart } from "@/modules/ledger/components/ledger-analysis-charts"
import { ChartLegend } from "@/components/charts/chart-parts"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

const topLevelValue = "__top__"

// LedgerAnalysisSpending shows the monthly expenses by top-level category, or
// by the subcategories of the top-level category parentId ("" for the top
// level), which the header's level select changes.
export function LedgerAnalysisSpending({ range, parentId, onParentChange }: {
  range: LedgerAnalysisRange
  parentId: string
  onParentChange: (parentId: string) => void
}) {
  const { t } = useTranslation()
  const { data: categories = [] } = useLedgerCategories()
  const query = useLedgerSpendingAnalysis(range, parentId || undefined)

  const parents = parentsWithChildren(categories)
  const parentName = categories.find((c) => c.id === parentId)?.name ?? ""

  const series = query.data
    ? spendingChartSeries(query.data, categories, {
        uncategorized: t("ledger.analysis.spending.uncategorized"),
        other: t("ledger.analysis.spending.other"),
        direct: t("ledger.analysis.spending.direct", { name: parentName }),
      })
    : []

  return (
    <LedgerAnalysisCard
      title={t("ledger.analysis.spending.title")}
      isPending={query.isPending}
      isError={query.isError}
      isRefreshing={query.isPlaceholderData}
      onRetry={() => void query.refetch()}
      isEmpty={series.length === 0}
      emptyText={t("ledger.analysis.spending.empty")}
      actions={
        <>
          {parentId && (
            <Button type="button" size="sm" variant="ghost" onClick={() => onParentChange("")}>
              {t("ledger.analysis.spending.allCategories")}
            </Button>
          )}
          {parents.length > 0 && (
            <Select value={parentId || topLevelValue} onValueChange={(value) => onParentChange(value === topLevelValue ? "" : value)}>
              <SelectTrigger size="sm" className="w-56" aria-label={t("ledger.analysis.spending.level")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={topLevelValue}>{t("ledger.analysis.spending.topLevel")}</SelectItem>
                {parents.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </>
      }
    >
      {query.data && (
        <>
          <SpendingChart months={query.data.months} series={series} />
          <ChartLegend items={series.map((s) => ({ label: s.label, color: s.color }))} />
        </>
      )}
    </LedgerAnalysisCard>
  )
}
