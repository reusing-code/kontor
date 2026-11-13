import { useTranslation } from "react-i18next"
import type { LedgerCategory, LedgerReviewQueueItem } from "@/modules/ledger/types"
import { formatAmountMinor, formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import { LedgerSuggestionBadge } from "@/modules/ledger/components/ledger-suggestion-badge"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export interface LedgerSuggestionSelection {
  isSelected: (transactionId: string) => boolean
  toggle: (transactionId: string) => void
  setAll: (selected: boolean) => void
  isMatchWordSelected: (transactionId: string) => boolean
  toggleMatchWord: (transactionId: string) => void
}

interface LedgerReviewListProps {
  transactions: LedgerReviewQueueItem[]
  categories: LedgerCategory[]
  selectedTransactionId?: string
  onSelect: (transaction: LedgerReviewQueueItem) => void
  nextCursor?: string
  onLoadMore?: () => void
  loadingMore?: boolean
  suggestions?: LedgerSuggestionSelection
}

export function LedgerReviewList({
  transactions,
  categories,
  selectedTransactionId,
  onSelect,
  nextCursor,
  onLoadMore,
  loadingMore = false,
  suggestions,
}: LedgerReviewListProps) {
  const { t, i18n } = useTranslation()
  const categoryNameById = new Map(categories.map((category) => [category.id, category.name]))
  const withSuggestion = suggestions ? transactions.filter((transaction) => transaction.suggestion) : []
  const showSelection = withSuggestion.length > 0
  const selectedCount = withSuggestion.filter((transaction) => suggestions?.isSelected(transaction.id)).length
  const allSelected: boolean | "indeterminate" = selectedCount === 0 ? false : selectedCount === withSuggestion.length ? true : "indeterminate"
  const columnCount = showSelection ? 6 : 5

  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              {showSelection && (
                <TableHead className="w-8">
                  <Checkbox
                    checked={allSelected}
                    onCheckedChange={(checked) => suggestions?.setAll(checked === true)}
                    aria-label={t("ledger.suggestions.selectAll")}
                  />
                </TableHead>
              )}
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
                <TableCell colSpan={columnCount} className="py-8 text-center text-muted-foreground">
                  {t("ledger.noTransactionsToReview")}
                </TableCell>
              </TableRow>
            ) : (
              transactions.map((transaction) => {
                const suggestion = suggestions ? transaction.suggestion : undefined
                return (
                  <TableRow
                    key={transaction.id}
                    className={selectedTransactionId === transaction.id ? "bg-accent/40" : undefined}
                    onClick={() => onSelect(transaction)}
                  >
                    {showSelection && (
                      <TableCell onClick={(event) => event.stopPropagation()}>
                        {suggestion && (
                          <Checkbox
                            checked={suggestions?.isSelected(transaction.id)}
                            onCheckedChange={() => suggestions?.toggle(transaction.id)}
                            aria-label={t("ledger.suggestions.select")}
                          />
                        )}
                      </TableCell>
                    )}
                    <TableCell>{formatLedgerDate(transaction.bookingDate)}</TableCell>
                    <TableCell className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                      {formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}
                    </TableCell>
                    <TableCell>{transaction.counterpartyName || "-"}</TableCell>
                    <TableCell>
                      {transaction.categoryId ? (
                        categoryNameById.get(transaction.categoryId) ?? transaction.categoryId
                      ) : suggestion ? (
                        <div className="space-y-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-muted-foreground italic">
                              {categoryNameById.get(suggestion.categoryId) ?? suggestion.categoryId}
                            </span>
                            <LedgerSuggestionBadge confidence={suggestion.confidence} />
                          </div>
                          {suggestion.matchWord && (
                            <label
                              className="flex items-center gap-2 text-xs text-muted-foreground"
                              onClick={(event) => event.stopPropagation()}
                            >
                              <Checkbox
                                checked={suggestions?.isMatchWordSelected(transaction.id)}
                                onCheckedChange={() => suggestions?.toggleMatchWord(transaction.id)}
                              />
                              {t("ledger.suggestions.addMatchWord", { word: suggestion.matchWord })}
                            </label>
                          )}
                        </div>
                      ) : (
                        t("ledger.noCategory")
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant={transaction.categorizationSource === "keyword" ? "default" : "secondary"}>
                        {t(`ledger.categorizationSource.${transaction.categorizationSource}`)}
                      </Badge>
                    </TableCell>
                  </TableRow>
                )
              })
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
