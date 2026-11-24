import { useState } from "react"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { formatDate } from "@/lib/format"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { todayISO } from "@/modules/ledger/lib/balance"
import {
  defaultAnalysisRange,
  resolveRange,
  type AnalysisRangeState,
} from "@/modules/ledger/lib/analysis-range"
import { useLedgerCashFlowAnalysis } from "@/modules/ledger/hooks/use-ledger-analysis"
import { LedgerAnalysisCashFlow } from "@/modules/ledger/components/ledger-analysis-cash-flow"
import { LedgerAnalysisNetWorth } from "@/modules/ledger/components/ledger-analysis-net-worth"
import { LedgerAnalysisRanking } from "@/modules/ledger/components/ledger-analysis-ranking"
import { LedgerAnalysisRangeControl } from "@/modules/ledger/components/ledger-analysis-range"
import { LedgerAnalysisSpending } from "@/modules/ledger/components/ledger-analysis-spending"

export function LedgerAnalysisPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.analysis.title"), t("app.title"))
  const today = todayISO()
  // state is what the control shows, applied the last valid range the
  // sections query: an invalid custom range keeps the data on screen.
  const [state, setState] = useState<AnalysisRangeState>(defaultAnalysisRange)
  const [applied, setApplied] = useState<LedgerAnalysisRange>(() => resolveRange(defaultAnalysisRange, todayISO()).range ?? {})

  const change = (next: AnalysisRangeState) => {
    setState(next)
    const { range } = resolveRange(next, today)
    if (range) setApplied(range)
  }

  // The cash-flow response (shared with its section through the query cache)
  // tells which dates "all" covers and which currencies were left out.
  const cashFlow = useLedgerCashFlowAnalysis(applied)
  const current = cashFlow.isPlaceholderData ? undefined : cashFlow.data
  const dates = (from: string, to: string) => ({
    from: formatDate(from),
    to: formatDate(to),
  })
  const active =
    applied.from && applied.to
      ? t("ledger.analysis.range.active", dates(applied.from, applied.to))
      : current
        ? t("ledger.analysis.range.allTimeDates", dates(current.from, current.to))
        : t("ledger.analysis.range.allTime")
  const excludedCurrencies = current?.excludedCurrencies ?? []
  // "Custom" starts with the dates on screen: for "all" those the data covers,
  // both capped at the local today (the server's UTC date can be a day ahead).
  const capped = (date: string) => (date < today ? date : today)
  const prefill: LedgerAnalysisRange =
    applied.from && applied.to ? applied : current ? { from: capped(current.from), to: capped(current.to) } : applied

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.analysis.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.analysis.description")}</p>
      </div>
      <div className="space-y-2">
        <LedgerAnalysisRangeControl state={state} prefill={prefill} today={today} onChange={change} />
        <p className="text-sm text-muted-foreground">{active}</p>
        {excludedCurrencies.length > 0 && (
          <p className="text-sm text-muted-foreground">
            {t("ledger.analysis.excludedCurrencies", { currencies: excludedCurrencies.join(", ") })}
          </p>
        )}
      </div>
      <LedgerAnalysisSpending range={applied} />
      <LedgerAnalysisCashFlow range={applied} />
      <LedgerAnalysisNetWorth range={applied} />
      <LedgerAnalysisRanking range={applied} />
    </div>
  )
}
