import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import type { LedgerCategory, LedgerTransaction } from "@/types/ledger"
import { transactionPath } from "@/lib/module-links"
import { formatAmountMinor, formatLedgerCategorizationSource, formatLedgerDate, formatLedgerReviewStatus, formatLedgerSpecialCategory, formatSourceType } from "@/lib/ledger-utils"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface LedgerTransactionsTableProps {
  transactions: LedgerTransaction[]
  categories?: LedgerCategory[]
  nextCursor?: string
  onLoadMore?: () => void
  loadingMore?: boolean
}

export function LedgerTransactionsTable({ transactions, categories = [], nextCursor, onLoadMore, loadingMore = false }: LedgerTransactionsTableProps) {
  const { t, i18n } = useTranslation()
  const categoryNameById = new Map(categories.map((category) => [category.id, category.name]))

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("ledger.date")}</TableHead>
              <TableHead>{t("ledger.valueDate")}</TableHead>
              <TableHead>{t("ledger.amount")}</TableHead>
              <TableHead>{t("ledger.counterparty")}</TableHead>
              <TableHead>{t("ledger.purpose")}</TableHead>
              <TableHead>{t("ledger.category")}</TableHead>
              <TableHead>{t("ledger.type")}</TableHead>
              <TableHead>{t("ledger.reviewState")}</TableHead>
              <TableHead>{t("ledger.source")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {transactions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={9} className="py-8 text-center text-muted-foreground">
                  {t("ledger.noTransactions")}
                </TableCell>
              </TableRow>
            ) : (
              transactions.map((txn) => (
                <TableRow key={txn.id}>
                  <TableCell>
                    <Link to={transactionPath(txn.id)} className="text-primary hover:underline">
                      {formatLedgerDate(txn.bookingDate)}
                    </Link>
                  </TableCell>
                  <TableCell>{formatLedgerDate(txn.valueDate)}</TableCell>
                  <TableCell className={txn.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                    {formatAmountMinor(txn.amountMinor, txn.currency, i18n.language)}
                  </TableCell>
                  <TableCell>{txn.counterpartyName || "-"}</TableCell>
                  <TableCell className="max-w-[24rem] truncate">{txn.purpose || "-"}</TableCell>
                  <TableCell>{txn.categoryId ? categoryNameById.get(txn.categoryId) ?? txn.categoryId : t("ledger.noCategory")}</TableCell>
                  <TableCell>{txn.transactionType || "-"}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-2">
                      {txn.specialCategory ? (
                        <Badge variant="secondary">{formatLedgerSpecialCategory(txn.specialCategory)}</Badge>
                      ) : null}
                      <Badge variant={txn.reviewStatus === "confirmed" ? "secondary" : "default"}>
                        {formatLedgerReviewStatus(txn.reviewStatus)}
                      </Badge>
                      <Badge variant="outline">{formatLedgerCategorizationSource(txn.categorizationSource)}</Badge>
                    </div>
                  </TableCell>
                  <TableCell>{formatSourceType(txn.sourceType)}</TableCell>
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
