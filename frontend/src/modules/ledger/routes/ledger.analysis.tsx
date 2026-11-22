import { useState } from "react"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { todayISO } from "@/modules/ledger/lib/balance"
import {
  defaultAnalysisRange,
  formatRangeDate,
  resolveRange,
  type AnalysisRangeState,
} from "@/modules/ledger/lib/analysis-range"
import { LedgerAnalysisRangeControl } from "@/modules/ledger/components/ledger-analysis-range"
import { LedgerAnalysisSpending } from "@/modules/ledger/components/ledger-analysis-spending"

export function LedgerAnalysisPage() {
  const { t, i18n } = useTranslation()
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

  const active =
    applied.from && applied.to
      ? t("ledger.analysis.range.active", {
          from: formatRangeDate(applied.from, i18n.language),
          to: formatRangeDate(applied.to, i18n.language),
        })
      : t("ledger.analysis.range.allTime")

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.analysis.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.analysis.description")}</p>
      </div>
      <div className="space-y-2">
        <LedgerAnalysisRangeControl state={state} applied={applied} today={today} onChange={change} />
        <p className="text-sm text-muted-foreground">{active}</p>
      </div>
      <LedgerAnalysisSpending range={applied} />
    </div>
  )
}
