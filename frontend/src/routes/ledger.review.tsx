import { useMemo, useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerCategories, useLedgerReviewQueue } from "@/hooks/use-ledger"
import { LedgerReviewList } from "@/components/ledger-review-list"
import { LedgerReviewPanel } from "@/components/ledger-review-panel"
import { rootRoute } from "./__root"

export const ledgerReviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/review",
  component: LedgerReviewPage,
})

function LedgerReviewPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.reviewQueue"), t("app.title"))
  const { data: categories = [] } = useLedgerCategories()
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const [selectedTransactionId, setSelectedTransactionId] = useState<string | undefined>()
  const cursor = cursorStack.length > 0 ? cursorStack[cursorStack.length - 1] : undefined
  const { data: page, isFetching } = useLedgerReviewQueue(50, cursor)

  const transactions = useMemo(() => page?.items ?? [], [page?.items])
  const selectedTransaction = useMemo(() => {
    if (transactions.length === 0) {
      return undefined
    }
    return transactions.find((item) => item.id === selectedTransactionId) ?? transactions[0]
  }, [selectedTransactionId, transactions])

  function handleReviewed(transactionId: string) {
    const currentIndex = transactions.findIndex((item) => item.id === transactionId)
    if (currentIndex < 0) {
      return
    }
    const nextTransaction = transactions[currentIndex + 1] ?? transactions[currentIndex - 1]
    setSelectedTransactionId(nextTransaction?.id)
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.reviewQueue")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.reviewQueueDescription")}</p>
      </div>

      <div className="grid gap-6 xl:grid-cols-[1.4fr_1fr]">
        <LedgerReviewList
          transactions={transactions}
          categories={categories}
          selectedTransactionId={selectedTransaction?.id}
          onSelect={(transaction) => setSelectedTransactionId(transaction.id)}
          nextCursor={page?.nextCursor}
          onLoadMore={page?.nextCursor ? () => setCursorStack((prev) => [...prev, page.nextCursor as string]) : undefined}
          loadingMore={isFetching}
        />
        <LedgerReviewPanel transaction={selectedTransaction} categories={categories} onReviewed={handleReviewed} />
      </div>
    </div>
  )
}
