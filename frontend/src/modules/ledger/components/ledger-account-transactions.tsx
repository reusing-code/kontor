import { useEffect, useRef } from "react"
import { useTranslation } from "react-i18next"
import { useLedgerTransactions } from "@/modules/ledger/hooks/use-ledger-transactions"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"
import type { LedgerCategory } from "@/modules/ledger/types"
import { Pager } from "@/components/pager"
import { pageCountOf } from "@/lib/pagination"
import { cn } from "@/lib/utils"

export const ledgerAccountPageSize = 100

interface LedgerAccountTransactionsProps {
  accountId: string
  categories: LedgerCategory[]
  currency?: string
  // 1-based
  page: number
  onPageChange: (page: number, options?: { replace?: boolean }) => void
}

export function LedgerAccountTransactions({ accountId, categories, currency, page, onPageChange }: LedgerAccountTransactionsProps) {
  const { t } = useTranslation()
  const listRef = useRef<HTMLDivElement>(null)
  const { data, isPending, isPlaceholderData } = useLedgerTransactions(accountId, ledgerAccountPageSize, (page - 1) * ledgerAccountPageSize)
  const pageCount = data ? pageCountOf(data.total, ledgerAccountPageSize) : 1

  // A page that no longer exists (fewer transactions, an old link) moves to the last one.
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
        <p className="text-sm text-muted-foreground">
          {isPending
            ? t("common.loading")
            : data && data.items.length > 0
              ? t("ledger.transactionRange", { from: data.offset + 1, to: data.offset + data.items.length, total: data.total })
              : null}
        </p>
        {pager(false)}
      </div>
      <div aria-busy={isPlaceholderData} className={cn("transition-opacity", isPlaceholderData && "opacity-60")}>
        <LedgerTransactionsTable transactions={data?.items ?? []} categories={categories} pageBalance={data?.pageBalance} currency={currency} />
      </div>
      <div className="flex justify-end">{pager(true)}</div>
    </div>
  )
}
