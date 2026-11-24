import { useEffect, useRef, type ReactNode } from "react"
import { useTranslation } from "react-i18next"
import { Pager } from "@/components/pager"
import { pageCountOf } from "@/lib/pagination"
import { cn } from "@/lib/utils"

export const ledgerTransactionPageSize = 100

interface LedgerTransactionPagesProps {
  // the loaded page: where it starts, how many items it has, and all matches
  data?: { offset: number; total: number; items: readonly unknown[] }
  isPending: boolean
  // the previous page is shown while this one loads
  isPlaceholderData: boolean
  // 1-based
  page: number
  onPageChange: (page: number, options?: { replace?: boolean }) => void
  // shown before the range text, e.g. a filter
  toolbar?: ReactNode
  children: ReactNode
}

// LedgerTransactionPages frames a server-paged transaction table: "from–to of
// total" and the pager above it, the pager below it (scrolling back to the
// top), the table dimmed while the next page loads, and a page past the end
// replaced by the last one.
export function LedgerTransactionPages({ data, isPending, isPlaceholderData, page, onPageChange, toolbar, children }: LedgerTransactionPagesProps) {
  const { t } = useTranslation()
  const listRef = useRef<HTMLDivElement>(null)
  const pageCount = data ? pageCountOf(data.total, ledgerTransactionPageSize) : 1

  useEffect(() => {
    if (data && !isPlaceholderData && page > pageCount) onPageChange(pageCount, { replace: true })
  }, [data, isPlaceholderData, page, pageCount, onPageChange])

  function goToPage(next: number, scroll: boolean) {
    onPageChange(next)
    if (scroll) listRef.current?.scrollIntoView?.({ block: "start" })
  }

  const pager = (scroll: boolean) => (
    <Pager page={Math.min(page, pageCount)} pageCount={pageCount} label={t("ledger.transactionPages")} onChange={(next) => goToPage(next, scroll)} />
  )

  return (
    <div ref={listRef} className="scroll-mt-6 space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-3">
          {toolbar}
          <p className="text-sm text-muted-foreground">
            {isPending
              ? t("common.loading")
              : data && data.items.length > 0
                ? t("ledger.transactionRange", { from: data.offset + 1, to: data.offset + data.items.length, total: data.total })
                : null}
          </p>
        </div>
        {pager(false)}
      </div>
      <div aria-busy={isPlaceholderData} className={cn("transition-opacity", isPlaceholderData && "opacity-60")}>
        {children}
      </div>
      <div className="flex justify-end">{pager(true)}</div>
    </div>
  )
}
