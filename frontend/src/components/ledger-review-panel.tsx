import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import type { LedgerCategory, LedgerReviewInput, LedgerTransaction } from "@/types/ledger"
import { useReviewLedgerTransaction } from "@/hooks/use-ledger"
import { formatAmountMinor, formatLedgerDate, tokenizeLedgerMatchWords } from "@/lib/ledger-utils"
import { LedgerTransferManager } from "@/components/ledger-transfer-manager"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Input } from "@/components/ui/input"

interface LedgerReviewPanelProps {
  transaction?: LedgerTransaction
  categories: LedgerCategory[]
  onReviewed?: (transactionId: string) => void
}

function LedgerReviewPanelInner({
  transaction,
  categories,
  onReviewed,
}: {
  transaction: LedgerTransaction
  categories: LedgerCategory[]
  onReviewed?: (transactionId: string) => void
}) {
  const { t, i18n } = useTranslation()
  const reviewMutation = useReviewLedgerTransaction()
  const isLinkedTransfer = Boolean(transaction.transferPairTransactionId)
  const [selectedCategoryId, setSelectedCategoryId] = useState<string | undefined>(transaction.categoryId)
  const [newCategoryName, setNewCategoryName] = useState("")
  const [matchWords, setMatchWords] = useState("")

  const suggestedWords = useMemo(() => {
    return tokenizeLedgerMatchWords(transaction.counterpartyName, transaction.purpose)
  }, [transaction.counterpartyName, transaction.purpose])

  const activeSelectedCategoryId = selectedCategoryId ?? transaction.categoryId ?? ""

  function submitReview(data: LedgerReviewInput) {
    reviewMutation.mutate({ id: transaction.id, data }, {
      onSuccess: () => {
        toast.success(t("ledger.transactionReviewed"))
        setNewCategoryName("")
        setMatchWords("")
        onReviewed?.(transaction.id)
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.reviewTransaction")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 md:grid-cols-2">
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.date")}</div>
            <div>{formatLedgerDate(transaction.bookingDate)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.amount")}</div>
            <div className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
              {formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.counterparty")}</div>
            <div>{transaction.counterpartyName || "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.purpose")}</div>
            <div>{transaction.purpose || "-"}</div>
          </div>
        </div>

        <div className="space-y-2">
          <div className="text-sm font-medium">{t("ledger.chooseCategory")}</div>
          <Select value={activeSelectedCategoryId || "none"} onValueChange={(value) => setSelectedCategoryId(value === "none" ? undefined : value)}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder={t("ledger.selectCategory")} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="none">{t("ledger.noCategory")}</SelectItem>
              {categories.map((category) => (
                <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2">
          <div className="text-sm font-medium">{t("ledger.createCategoryInline")}</div>
          <Input value={newCategoryName} onChange={(event) => setNewCategoryName(event.target.value)} placeholder={t("ledger.categoryNamePlaceholder")} />
        </div>

        <div className="space-y-2">
          <div className="text-sm font-medium">{t("ledger.quickMatchWords")}</div>
          <div className="flex flex-wrap gap-2">
            {suggestedWords.map((word) => (
              <Button
                key={word}
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setMatchWords((current) => current ? `${current}, ${word}` : word)}
              >
                {word}
              </Button>
            ))}
          </div>
          <Input value={matchWords} onChange={(event) => setMatchWords(event.target.value)} placeholder={t("ledger.matchWordsPlaceholder")} />
        </div>

        <div className="flex flex-wrap gap-2">
          {transaction.categoryId && (
            <Badge variant="outline">{t("ledger.suggestedCategory")}</Badge>
          )}
          {isLinkedTransfer && (
            <Badge variant="outline">{t("ledger.transferLinked")}</Badge>
          )}
          <Badge variant={transaction.categorizationSource === "keyword" ? "default" : "secondary"}>
            {t(`ledger.categorizationSource.${transaction.categorizationSource}`)}
          </Badge>
        </div>

        {isLinkedTransfer ? (
          <div className="space-y-3 rounded-md border p-4">
            <div>
              <div className="text-sm font-medium">{t("ledger.internalTransfer")}</div>
              <p className="text-sm text-muted-foreground">{t("ledger.transferReviewProtection")}</p>
            </div>
            <LedgerTransferManager transaction={transaction} compact />
          </div>
        ) : null}

        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            onClick={() => submitReview({
                categoryId: activeSelectedCategoryId || transaction.categoryId,
                addMatchWords: matchWords.split(",").map((word) => word.trim()).filter(Boolean),
              })}
            disabled={reviewMutation.isPending || isLinkedTransfer}
          >
            {t("ledger.confirmCategory")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => submitReview({
                newCategory: newCategoryName ? { name: newCategoryName, matchWords: [], parentId: undefined } : undefined,
                categoryId: newCategoryName ? undefined : activeSelectedCategoryId || undefined,
                addMatchWords: matchWords.split(",").map((word) => word.trim()).filter(Boolean),
              })}
            disabled={reviewMutation.isPending || isLinkedTransfer || (!newCategoryName && !activeSelectedCategoryId)}
          >
            {t("ledger.assignCategory")}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}

export function LedgerReviewPanel({ transaction, categories, onReviewed }: LedgerReviewPanelProps) {
  const { t } = useTranslation()

  if (!transaction) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.review")}</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">{t("ledger.noReviewSelection")}</p>
        </CardContent>
      </Card>
    )
  }

  return <LedgerReviewPanelInner key={transaction.id} transaction={transaction} categories={categories} onReviewed={onReviewed} />
}
