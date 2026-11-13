import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { SparklesIcon } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useCapabilities } from "@/hooks/use-capabilities"
import {
  useLedgerCategories,
  useLedgerReviewQueue,
  useLedgerSuggestionStatus,
  useReviewLedgerTransactionsBatch,
  useRunLedgerCategorySuggestions,
} from "@/modules/ledger/hooks/use-ledger"
import { LedgerReviewList, type LedgerSuggestionSelection } from "@/modules/ledger/components/ledger-review-list"
import { LedgerReviewPanel } from "@/modules/ledger/components/ledger-review-panel"
import type { LedgerSuggestionRunResult } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"

function toggled(set: Set<string>, id: string): Set<string> {
  const next = new Set(set)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  return next
}

export function LedgerReviewPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.reviewQueue"), t("app.title"))
  const { llm } = useCapabilities()
  const { data: categories = [] } = useLedgerCategories()
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const [selectedTransactionId, setSelectedTransactionId] = useState<string | undefined>()
  const cursor = cursorStack.length > 0 ? cursorStack[cursorStack.length - 1] : undefined
  const { data: page, isFetching } = useLedgerReviewQueue(50, cursor)
  const suggestionsRunning = useLedgerSuggestionStatus(llm)
  const runSuggestions = useRunLedgerCategorySuggestions()
  const batchReview = useReviewLedgerTransactionsBatch()
  // Every suggestion starts selected, so only opt-outs are tracked; match
  // words start off, so only opt-ins are tracked.
  const [deselected, setDeselected] = useState<Set<string>>(() => new Set())
  const [matchWordsOn, setMatchWordsOn] = useState<Set<string>>(() => new Set())

  const transactions = useMemo(() => page?.items ?? [], [page?.items])
  const selectedTransaction = useMemo(() => {
    if (transactions.length === 0) {
      return undefined
    }
    return transactions.find((item) => item.id === selectedTransactionId) ?? transactions[0]
  }, [selectedTransactionId, transactions])

  const withSuggestion = useMemo(
    () => (llm ? transactions.filter((item) => item.suggestion && !item.categoryId) : []),
    [llm, transactions],
  )
  const selectedSuggestions = withSuggestion.filter((item) => !deselected.has(item.id))

  const selection: LedgerSuggestionSelection = {
    isSelected: (id) => !deselected.has(id),
    toggle: (id) => setDeselected((current) => toggled(current, id)),
    setAll: (selected) => setDeselected(selected ? new Set() : new Set(withSuggestion.map((item) => item.id))),
    isMatchWordSelected: (id) => matchWordsOn.has(id),
    toggleMatchWord: (id) => setMatchWordsOn((current) => toggled(current, id)),
  }

  function handleReviewed(transactionId: string) {
    const currentIndex = transactions.findIndex((item) => item.id === transactionId)
    if (currentIndex < 0) {
      return
    }
    const nextTransaction = transactions[currentIndex + 1] ?? transactions[currentIndex - 1]
    setSelectedTransactionId(nextTransaction?.id)
  }

  function runSummary(result: LedgerSuggestionRunResult): string {
    const parts = [t("ledger.suggestions.runSuggested", { count: result.suggested })]
    if (result.ruleMatched > 0) {
      parts.push(t("ledger.suggestions.runRuleMatched", { count: result.ruleMatched }))
    }
    if (result.noSuggestion > 0) {
      parts.push(t("ledger.suggestions.runNoSuggestion", { count: result.noSuggestion }))
    }
    if (result.remaining > 0) {
      parts.push(t("ledger.suggestions.runRemaining", { count: result.remaining }))
    }
    return parts.join(" ")
  }

  function handleRunSuggestions() {
    runSuggestions.mutate(undefined, {
      onSuccess: (result) => {
        if (result.candidates === 0 && result.ruleMatched === 0) {
          toast.info(t("ledger.suggestions.runNothing"))
        } else if (result.partial || result.warnings.length > 0) {
          toast.warning(runSummary(result), { description: t("ledger.suggestions.runIncomplete") })
        } else {
          toast.success(runSummary(result))
        }
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleAcceptSelected() {
    const items = selectedSuggestions.flatMap((item) => item.suggestion ? [{
      transactionId: item.id,
      categoryId: item.suggestion.categoryId,
      addMatchWords: item.suggestion.matchWord && matchWordsOn.has(item.id) ? [item.suggestion.matchWord] : [],
    }] : [])
    batchReview.mutate(items, {
      onSuccess: (result) => {
        if (result.reviewed > 0) {
          toast.success(t("ledger.suggestions.accepted", { count: result.reviewed }))
        }
        if (result.failed.length > 0) {
          const codes = [...new Set(result.failed.map((failure) => t(`ledger.suggestions.failure.${failure.code}`)))]
          toast.warning(t("ledger.suggestions.acceptFailed", { count: result.failed.length }), { description: codes.join(", ") })
        }
      },
      onError: (error) => toast.error(error.message),
    })
  }

  const suggesting = suggestionsRunning || runSuggestions.isPending

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{t("ledger.reviewQueue")}</h1>
          <p className="text-sm text-muted-foreground">{t("ledger.reviewQueueDescription")}</p>
        </div>
        {llm && (
          <Button type="button" variant="outline" onClick={handleRunSuggestions} disabled={suggesting}>
            <SparklesIcon />
            {suggesting ? t("ledger.suggestions.running") : t("ledger.suggestions.run")}
          </Button>
        )}
      </div>

      {withSuggestion.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/40 px-4 py-3">
          <p className="text-sm text-muted-foreground">
            {t("ledger.suggestions.selectedCount", { count: selectedSuggestions.length, total: withSuggestion.length })}
          </p>
          <Button
            type="button"
            onClick={handleAcceptSelected}
            disabled={selectedSuggestions.length === 0 || batchReview.isPending}
          >
            {t("ledger.suggestions.acceptSelected", { count: selectedSuggestions.length })}
          </Button>
        </div>
      )}

      <div className="grid gap-6 xl:grid-cols-[1.4fr_1fr]">
        <LedgerReviewList
          transactions={transactions}
          categories={categories}
          selectedTransactionId={selectedTransaction?.id}
          onSelect={(transaction) => setSelectedTransactionId(transaction.id)}
          nextCursor={page?.nextCursor}
          onLoadMore={page?.nextCursor ? () => setCursorStack((prev) => [...prev, page.nextCursor as string]) : undefined}
          loadingMore={isFetching}
          suggestions={llm ? selection : undefined}
        />
        <LedgerReviewPanel
          transaction={selectedTransaction}
          categories={categories}
          onReviewed={handleReviewed}
          showSuggestion={llm}
        />
      </div>
    </div>
  )
}
