import { useTranslation } from "react-i18next"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { useLedgerCashFlowAnalysis } from "@/modules/ledger/hooks/use-ledger-analysis"
import { cashFlowSeries } from "@/modules/ledger/lib/analysis-series"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"
import { CashFlowChart } from "@/modules/ledger/components/ledger-analysis-charts"
import { ChartLegend } from "@/components/charts/chart-parts"

// LedgerAnalysisCashFlow shows income, expenses and the net result per month.
export function LedgerAnalysisCashFlow({ range }: { range: LedgerAnalysisRange }) {
  const { t } = useTranslation()
  const query = useLedgerCashFlowAnalysis(range)
  const data = query.data
  const series = data
    ? cashFlowSeries(data, {
        income: t("ledger.analysis.cashFlow.income"),
        expenses: t("ledger.analysis.cashFlow.expenses"),
        net: t("ledger.analysis.cashFlow.net"),
      })
    : []
  const isEmpty = !data || data.months.every((_, month) => data.incomeMinor[month] === 0 && data.expensesMinor[month] === 0)

  return (
    <LedgerAnalysisCard
      title={t("ledger.analysis.cashFlow.title")}
      isPending={query.isPending}
      isError={query.isError}
      isRefreshing={query.isPlaceholderData}
      onRetry={() => void query.refetch()}
      isEmpty={isEmpty}
      emptyText={t("ledger.analysis.cashFlow.empty")}
    >
      {data && (
        <>
          <CashFlowChart months={data.months} series={series} />
          <ChartLegend items={series.map((s) => ({ label: s.label, color: s.color }))} />
        </>
      )}
    </LedgerAnalysisCard>
  )
}
