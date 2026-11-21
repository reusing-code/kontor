import { useMemo, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ChevronLeftIcon, ChevronRightIcon, ChevronsLeftIcon, ChevronsRightIcon, SparklesIcon } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useCapabilities } from "@/hooks/use-capabilities"
import {
  useLedgerCategories,
  useLedgerReviewQueueAll,
  useLedgerSuggestionStatus,
  useReviewLedgerTransactionsBatch,
  useRunLedgerCategorySuggestions,
} from "@/modules/ledger/hooks/use-ledger"
import { LedgerReviewList, type LedgerReviewSelection } from "@/modules/ledger/components/ledger-review-list"
import { LedgerReviewPanel } from "@/modules/ledger/components/ledger-review-panel"
import {
  batchReviewItems,
  categoryOverrides,
  entryItems,
  groupReviewQueue,
  reviewProposal,
  type ReviewEntry,
} from "@/modules/ledger/lib/review-queue"
import type { LedgerReviewQueueItem, LedgerSuggestionRunResult } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"

// Entries (single transactions or groups) per page.
export const reviewPageSize = 50

function withIds(set: ReadonlySet<string>, ids: string[], on: boolean): ReadonlySet<string> {
  const next = new Set(set)
  ids.forEach((id) => (on ? next.add(id) : next.delete(id)))
  return next
}

function visibleRows(entries: ReviewEntry[], expanded: ReadonlySet<string>): LedgerReviewQueueItem[] {
  return entries.flatMap((entry) => {
    if (entry.kind === "single") return [entry.item]
    return expanded.has(entry.key) ? entry.items : []
  })
}

interface PagerProps {
  page: number
  pageCount: number
  onChange: (page: number) => void
}

function Pager({ page, pageCount, onChange }: PagerProps) {
  const { t } = useTranslation()
  if (pageCount <= 1) return null
  const buttons = [
    { label: t("ledger.queue.firstPage"), icon: <ChevronsLeftIcon />, target: 0, disabled: page === 0 },
    { label: t("ledger.queue.previousPage"), icon: <ChevronLeftIcon />, target: page - 1, disabled: page === 0 },
    { label: t("ledger.queue.nextPage"), icon: <ChevronRightIcon />, target: page + 1, disabled: page >= pageCount - 1 },
    { label: t("ledger.queue.lastPage"), icon: <ChevronsRightIcon />, target: pageCount - 1, disabled: page >= pageCount - 1 },
  ]
  const button = (b: (typeof buttons)[number]) => (
    <Button key={b.label} type="button" variant="outline" size="icon" className="size-8" aria-label={b.label} disabled={b.disabled} onClick={() => onChange(b.target)}>
      {b.icon}
    </Button>
  )
  return (
    <nav className="flex items-center gap-1 text-sm" aria-label={t("ledger.queue.pagination")}>
      {buttons.slice(0, 2).map(button)}
      <span className="px-2 tabular-nums">{t("ledger.queue.pageInfo", { page: page + 1, pages: pageCount })}</span>
      {buttons.slice(2).map(button)}
    </nav>
  )
}

export function LedgerReviewPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.reviewQueue"), t("app.title"))
  const { llm } = useCapabilities()
  const { data: categories = [] } = useLedgerCategories()
  const { data: queue, isLoading } = useLedgerReviewQueueAll()
  const suggestionsRunning = useLedgerSuggestionStatus(llm)
  const runSuggestions = useRunLedgerCategorySuggestions()
  const batchReview = useReviewLedgerTransactionsBatch()
  const listRef = useRef<HTMLDivElement>(null)

  const [selectedTransactionId, setSelectedTransactionId] = useState<string | undefined>()
  const [page, setPage] = useState(0)
  const [expandedGroups, setExpandedGroups] = useState<ReadonlySet<string>>(() => new Set())
  // Every proposal starts selected, so only opt-outs are tracked; match words
  // start off, so only opt-ins are tracked.
  const [deselected, setDeselected] = useState<ReadonlySet<string>>(() => new Set())
  const [matchWordsOn, setMatchWordsOn] = useState<ReadonlySet<string>>(() => new Set())
  const [groupCategories, setGroupCategories] = useState<ReadonlyMap<string, string>>(() => new Map())

  const transactions = useMemo(() => queue?.items ?? [], [queue?.items])
  const entries = useMemo(() => groupReviewQueue(transactions, llm), [transactions, llm])
  const pageCount = Math.max(1, Math.ceil(entries.length / reviewPageSize))
  // Reviewing shrinks the queue; stay on the last page that still exists.
  const currentPage = Math.min(page, pageCount - 1)
  const pageEntries = entries.slice(currentPage * reviewPageSize, (currentPage + 1) * reviewPageSize)
  const selectedTransaction = transactions.find((item) => item.id === selectedTransactionId)

  const withProposal = useMemo(() => transactions.filter((item) => reviewProposal(item, llm)), [transactions, llm])
  const selectedItems = withProposal.filter((item) => !deselected.has(item.id))

  const selection: LedgerReviewSelection = {
    isSelected: (id) => !deselected.has(id),
    setSelected: (ids, on) => setDeselected((current) => withIds(current, ids, !on)),
    isMatchWordSelected: (id) => matchWordsOn.has(id),
    setMatchWord: (ids, on) => setMatchWordsOn((current) => withIds(current, ids, on)),
  }

  function goToPage(next: number, scroll: boolean) {
    setPage(next)
    if (scroll) listRef.current?.scrollIntoView?.({ block: "start" })
  }

  // Picking a category for a group also selects the group for accepting.
  function setGroupCategory(key: string, categoryId: string | undefined) {
    setGroupCategories((current) => {
      const next = new Map(current)
      if (categoryId) next.set(key, categoryId)
      else next.delete(key)
      return next
    })
    const group = entries.find((entry) => entry.kind === "group" && entry.key === key)
    if (categoryId && group) selection.setSelected(entryItems(group).map((item) => item.id), true)
  }

  function toggleGroup(key: string) {
    setExpandedGroups((current) => withIds(current, [key], !current.has(key)))
  }

  function handleReviewed(transactionId: string) {
    const rows = visibleRows(pageEntries, expandedGroups)
    const index = rows.findIndex((item) => item.id === transactionId)
    setSelectedTransactionId(index < 0 ? undefined : (rows[index + 1] ?? rows[index - 1])?.id)
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
    const items = batchReviewItems(selectedItems, llm, (id) => matchWordsOn.has(id), categoryOverrides(entries, groupCategories))
    batchReview.mutate(items, {
      onSuccess: (result) => {
        if (result.reviewed > 0) {
          toast.success(t("ledger.queue.accepted", { count: result.reviewed }))
        }
        if (result.failed.length > 0) {
          const codes = [...new Set(result.failed.map((failure) => t(`ledger.queue.failure.${failure.code}`)))]
          toast.warning(t("ledger.queue.acceptFailed", { count: result.failed.length }), { description: codes.join(", ") })
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

      {withProposal.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/40 px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-sm text-muted-foreground">
              {t("ledger.queue.selectedCount", { count: selectedItems.length, total: withProposal.length })}
            </p>
            <Button type="button" variant="ghost" size="sm" onClick={() => setDeselected(new Set())}>
              {t("ledger.queue.selectAll")}
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={() => setDeselected(new Set(withProposal.map((item) => item.id)))}>
              {t("ledger.queue.selectNone")}
            </Button>
          </div>
          <Button
            type="button"
            onClick={handleAcceptSelected}
            disabled={selectedItems.length === 0 || batchReview.isPending}
          >
            {t("ledger.queue.acceptSelected", { count: selectedItems.length })}
          </Button>
        </div>
      )}

      <div className="grid items-start gap-6 xl:grid-cols-[1.4fr_1fr]">
        <div ref={listRef} className="min-w-0 scroll-mt-6 space-y-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm text-muted-foreground">
              {isLoading ? t("common.loading") : t("ledger.queue.count", { count: transactions.length })}
            </p>
            <Pager page={currentPage} pageCount={pageCount} onChange={(next) => goToPage(next, false)} />
          </div>
          {!isLoading && (
            <LedgerReviewList
              entries={pageEntries}
              categories={categories}
              llm={llm}
              selectedTransactionId={selectedTransaction?.id}
              onSelect={(item) => setSelectedTransactionId(item.id)}
              expandedGroups={expandedGroups}
              onToggleGroup={toggleGroup}
              groupCategory={(key) => groupCategories.get(key)}
              onGroupCategoryChange={setGroupCategory}
              selection={selection}
              showSelection={withProposal.length > 0}
            />
          )}
          <div className="flex justify-end">
            <Pager page={currentPage} pageCount={pageCount} onChange={(next) => goToPage(next, true)} />
          </div>
        </div>
        <div className="xl:sticky xl:top-6 xl:max-h-[calc(100vh-3rem)] xl:overflow-y-auto">
          <LedgerReviewPanel
            transaction={selectedTransaction}
            categories={categories}
            onReviewed={handleReviewed}
            onClose={() => setSelectedTransactionId(undefined)}
            showSuggestion={llm}
          />
        </div>
      </div>
    </div>
  )
}
