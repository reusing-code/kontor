import { useState } from "react"
import { useTranslation } from "react-i18next"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { useLedgerNetWorthAnalysis } from "@/modules/ledger/hooks/use-ledger-analysis"
import { netWorthSeries } from "@/modules/ledger/lib/analysis-series"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"
import { NetWorthChart } from "@/modules/ledger/components/ledger-analysis-charts"
import { ChartLegend } from "@/components/charts/chart-parts"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"

// LedgerAnalysisNetWorth shows the total of the account balances over time,
// optionally one line per account, and names the accounts that do not count.
export function LedgerAnalysisNetWorth({ range }: { range: LedgerAnalysisRange }) {
  const { t } = useTranslation()
  const [showAccounts, setShowAccounts] = useState(false)
  const query = useLedgerNetWorthAnalysis(range)
  const data = query.data
  const series = data ? netWorthSeries(data, t("ledger.analysis.netWorth.total"), showAccounts) : []
  const excluded = (data?.excludedAccounts ?? []).map((account) => {
    const reason =
      account.reason === "currency"
        ? t("ledger.analysis.netWorth.reasonCurrency", { currency: account.currency ?? "" })
        : t("ledger.analysis.netWorth.reasonNoBalance")
    return `${account.name} (${reason})`
  })

  return (
    <LedgerAnalysisCard
      title={t("ledger.analysis.netWorth.title")}
      isPending={query.isPending}
      isError={query.isError}
      isRefreshing={query.isPlaceholderData}
      onRetry={() => void query.refetch()}
      isEmpty={!data || data.dates.length === 0}
      emptyText={t("ledger.analysis.netWorth.empty")}
      actions={
        <div className="flex items-center gap-2">
          <Switch id="ledger-analysis-show-accounts" checked={showAccounts} onCheckedChange={setShowAccounts} />
          <Label htmlFor="ledger-analysis-show-accounts">{t("ledger.analysis.netWorth.showAccounts")}</Label>
        </div>
      }
      footer={
        excluded.length > 0 && (
          <p className="mt-3 text-sm text-muted-foreground">{t("ledger.analysis.netWorth.excluded", { accounts: excluded.join(", ") })}</p>
        )
      }
    >
      {data && (
        <>
          <NetWorthChart dates={data.dates} series={series} />
          <ChartLegend items={series.map((s) => ({ label: s.label, color: s.color }))} />
        </>
      )}
    </LedgerAnalysisCard>
  )
}
