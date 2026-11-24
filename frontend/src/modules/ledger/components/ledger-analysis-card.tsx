import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface LedgerAnalysisCardProps {
  title: string
  isPending: boolean
  isError: boolean
  // the content is still that of the previous range while the new one loads
  isRefreshing?: boolean
  onRetry: () => void
  isEmpty: boolean
  emptyText: string
  // shown in the header next to the title
  actions?: ReactNode
  // shown below the content, also in the empty state
  footer?: ReactNode
  children?: ReactNode
}

// LedgerAnalysisCard frames one analysis and shows its loading, error and
// empty state instead of the content; content of the previous range is dimmed.
export function LedgerAnalysisCard({ title, isPending, isError, isRefreshing = false, onRetry, isEmpty, emptyText, actions, footer, children }: LedgerAnalysisCardProps) {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle>{title}</CardTitle>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </CardHeader>
      <CardContent>
        {isError ? (
          <div role="alert" className="flex flex-wrap items-center gap-3 text-sm">
            <span className="text-destructive">{t("ledger.analysis.error")}</span>
            <Button type="button" size="sm" variant="outline" onClick={onRetry}>
              {t("ledger.analysis.retry")}
            </Button>
          </div>
        ) : isPending ? (
          <output aria-label={t("common.loading")} className="block h-[280px] animate-pulse rounded-md bg-muted" />
        ) : (
          <div aria-busy={isRefreshing} className={cn("transition-opacity", isRefreshing && "opacity-60")}>
            {isEmpty ? <p className="text-sm text-muted-foreground">{emptyText}</p> : children}
            {footer}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
