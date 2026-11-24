import { formatDate, formatMinor } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Copy } from "lucide-react"
import { useReviewLedgerTransaction } from "@/modules/ledger/hooks/use-ledger-review"
import { LedgerCategorySelect } from "@/modules/ledger/components/ledger-category-select"
import { formatLedgerReviewStatus, formatLedgerSpecialCategory } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerCategory, LedgerTransaction } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function LedgerTransactionFieldsCard({ transaction, categories }: {
  transaction: LedgerTransaction
  categories: LedgerCategory[]
}) {
  const { t } = useTranslation()
  const reviewMutation = useReviewLedgerTransaction()

  const categoryName = transaction.categoryId ? categories.find((category) => category.id === transaction.categoryId)?.name : undefined

  function handleCopyLink() {
    navigator.clipboard.writeText(window.location.href)
      .then(() => toast.success(t("ledger.transactionLinkCopied")))
      .catch(() => toast.error(t("ledger.transactionLinkCopyFailed")))
  }

  // Changing the category here reviews the transaction with it, like the review queue.
  function handleCategoryChange(categoryId: string) {
    if (!categoryId || categoryId === transaction.categoryId) return
    reviewMutation.mutate({ id: transaction.id, data: { categoryId, addMatchWords: [] } }, {
      onSuccess: () => toast.success(t("ledger.categoryChanged")),
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <CardTitle>{t("ledger.transactionDetails")}</CardTitle>
          <Button type="button" variant="outline" size="sm" onClick={handleCopyLink}>
            <Copy className="mr-2 h-4 w-4" />
            {t("ledger.copyTransactionLink")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 md:grid-cols-2">
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.date")}</div>
          <div>{formatDate(transaction.bookingDate)}</div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.valueDate")}</div>
          <div>{transaction.valueDate ? formatDate(transaction.valueDate) : "-"}</div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.amount")}</div>
          <div className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
            {formatMinor(transaction.amountMinor, transaction.currency)}
          </div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.reviewState")}</div>
          <div>{formatLedgerReviewStatus(transaction.reviewStatus)}</div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.counterparty")}</div>
          <div>{transaction.counterpartyName || "-"}</div>
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.category")}</div>
          {transaction.transferPairTransactionId ? (
            <div>{categoryName ?? t("ledger.noCategory")}</div>
          ) : (
            <LedgerCategorySelect
              categories={categories}
              value={transaction.categoryId ?? ""}
              onValueChange={handleCategoryChange}
              placeholder={t("ledger.noCategory")}
              disabled={reviewMutation.isPending}
              aria-label={t("ledger.category")}
            />
          )}
        </div>
        <div>
          <div className="text-xs text-muted-foreground">{t("ledger.specialCategory")}</div>
          <div>{transaction.specialCategory ? formatLedgerSpecialCategory(transaction.specialCategory) : "-"}</div>
        </div>
        <div className="md:col-span-2">
          <div className="text-xs text-muted-foreground">{t("ledger.purpose")}</div>
          <div className="whitespace-pre-wrap">{transaction.purpose || "-"}</div>
        </div>
      </CardContent>
    </Card>
  )
}
