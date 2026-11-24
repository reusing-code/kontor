import { useTranslation } from "react-i18next"
import { formatMinor } from "@/lib/format"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"
import { useLedgerRankingAnalysis } from "@/modules/ledger/hooks/use-ledger"
import { LedgerAnalysisCard } from "@/modules/ledger/components/ledger-analysis-card"

interface RankedEntry {
  key: string
  name: string
  context?: string
  amountMinor: number
}

// The entries come sorted, so the first one is the widest bar.
function RankedList({ entries }: { entries: RankedEntry[] }) {
  const largest = entries[0]?.amountMinor ?? 0

  return (
    <ol className="space-y-3">
      {entries.map((entry) => (
        <li key={entry.key} className="space-y-1">
          <div className="flex items-baseline justify-between gap-4 text-sm">
            <span className="truncate">
              {entry.context && <span className="text-muted-foreground">{entry.context} › </span>}
              {entry.name}
            </span>
            <span className="font-medium tabular-nums">{formatMinor(entry.amountMinor)}</span>
          </div>
          <div className="h-1.5 rounded-full bg-primary/20" style={{ width: `${largest > 0 ? Math.max((entry.amountMinor / largest) * 100, 1) : 0}%` }} />
        </li>
      ))}
    </ol>
  )
}

// LedgerAnalysisRanking lists the categories and counterparties with the
// highest spending in the range.
export function LedgerAnalysisRanking({ range }: { range: LedgerAnalysisRange }) {
  const { t } = useTranslation()
  const query = useLedgerRankingAnalysis(range)
  const categories: RankedEntry[] = (query.data?.categories ?? []).map((c) => ({
    key: c.categoryId ?? c.kind,
    name: c.kind === "uncategorized" ? t("ledger.analysis.spending.uncategorized") : c.name,
    context: c.parentName,
    amountMinor: c.amountMinor,
  }))
  const counterparties: RankedEntry[] = (query.data?.counterparties ?? []).map((c) => ({
    // the entry without a name is the only one with an empty key
    key: c.name,
    name: c.unknown ? t("ledger.analysis.ranking.unknownCounterparty") : c.name,
    amountMinor: c.amountMinor,
  }))
  const lists = [
    { title: t("ledger.analysis.ranking.categories"), entries: categories },
    { title: t("ledger.analysis.ranking.counterparties"), entries: counterparties },
  ]

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {lists.map((list) => (
        <LedgerAnalysisCard
          key={list.title}
          title={list.title}
          isPending={query.isPending}
          isError={query.isError}
          isRefreshing={query.isPlaceholderData}
          onRetry={() => void query.refetch()}
          isEmpty={list.entries.length === 0}
          emptyText={t("ledger.analysis.ranking.empty")}
        >
          <RankedList entries={list.entries} />
        </LedgerAnalysisCard>
      ))}
    </div>
  )
}
