import { useTranslation } from "react-i18next"
import type { LedgerCategory, LedgerTransaction } from "@/types/ledger"
import { formatAmountMinor, formatLedgerDate } from "@/lib/ledger-utils"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface LedgerReviewListProps {
  transactions: LedgerTransaction[]
  categories: LedgerCategory[]
  selectedTransactionId?: string
  onSelect: (transaction: LedgerTransaction) => void
  nextCursor?: string
  onLoadMore?: () => void
  loadingMore?: boolean
}

export function LedgerReviewList({
  transactions,
  categories,
  selectedTransactionId,
  onSelect,
  nextCursor,
  onLoadMore,
  loadingMore = false,
}: LedgerReviewListProps) {
  const { t, i18n } = useTranslation()
  const categoryNameById = new Map(categories.map((category) => [category.id, category.name]))

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("ledger.date")}</TableHead>
              <TableHead>{t("ledger.amount")}</TableHead>
              <TableHead>{t("ledger.counterparty")}</TableHead>
              <TableHead>{t("ledger.category")}</TableHead>
              <TableHead>{t("ledger.status")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {transactions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                  {t("ledger.noTransactionsToReview")}
                </TableCell>
              </TableRow>
            ) : (
              transactions.map((transaction) => (
                <TableRow
                  key={transaction.id}
                  className={selectedTransactionId === transaction.id ? "bg-accent/40" : undefined}
                  onClick={() => onSelect(transaction)}
                >
                  <TableCell>{formatLedgerDate(transaction.bookingDate)}</TableCell>
                  <TableCell className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                    {formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}
                  </TableCell>
                  <TableCell>{transaction.counterpartyName || "-"}</TableCell>
                  <TableCell>{transaction.categoryId ? categoryNameById.get(transaction.categoryId) ?? transaction.categoryId : t("ledger.noCategory")}</TableCell>
                  <TableCell>
                    <Badge variant={transaction.categorizationSource === "keyword" ? "default" : "secondary"}>
                      {t(`ledger.categorizationSource.${transaction.categorizationSource}`)}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {nextCursor && onLoadMore && (
        <div className="flex justify-center">
          <Button type="button" variant="outline" onClick={onLoadMore} disabled={loadingMore}>
            {loadingMore ? t("ledger.loadingMore") : t("ledger.loadMore")}
          </Button>
        </div>
      )}
    </div>
  )
}
