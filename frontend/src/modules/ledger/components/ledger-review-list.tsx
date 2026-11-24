import type { SyntheticEvent } from "react"
import { formatDate, formatMinor } from "@/lib/format"
import { ChevronDownIcon, ChevronRightIcon, Undo2Icon } from "lucide-react"
import { useTranslation } from "react-i18next"
import type { LedgerCategory, LedgerReviewQueueItem } from "@/modules/ledger/types"
import { entryItems, reviewProposal, type ReviewEntry, type ReviewProposal } from "@/modules/ledger/lib/review-queue"
import { LedgerSuggestionBadge } from "@/modules/ledger/components/ledger-suggestion-badge"
import { LedgerCategorySelect } from "@/modules/ledger/components/ledger-category-select"
import { LedgerSplitBadge } from "@/modules/ledger/components/ledger-split-badge"
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
import { cn } from "@/lib/utils"

export interface LedgerReviewSelection {
  isSelected: (transactionId: string) => boolean
  setSelected: (transactionIds: string[], selected: boolean) => void
  isMatchWordSelected: (transactionId: string) => boolean
  setMatchWord: (transactionIds: string[], selected: boolean) => void
}

interface LedgerReviewListProps {
  entries: ReviewEntry[]
  categories: LedgerCategory[]
  llm: boolean
  selectedTransactionId?: string
  onSelect: (transaction: LedgerReviewQueueItem) => void
  expandedGroups: ReadonlySet<string>
  onToggleGroup: (key: string) => void
  // category chosen on a group row instead of its proposal
  groupCategory: (key: string) => string | undefined
  onGroupCategoryChange: (key: string, categoryId: string | undefined) => void
  selection: LedgerReviewSelection
  showSelection: boolean
}

function checkedState(ids: string[], isOn: (id: string) => boolean): boolean | "indeterminate" {
  const on = ids.filter(isOn).length
  return on === 0 ? false : on === ids.length ? true : "indeterminate"
}

function stopPropagation(event: SyntheticEvent) {
  event.stopPropagation()
}

export function LedgerReviewList({
  entries,
  categories,
  llm,
  selectedTransactionId,
  onSelect,
  expandedGroups,
  onToggleGroup,
  groupCategory,
  onGroupCategoryChange,
  selection,
  showSelection,
}: LedgerReviewListProps) {
  const { t } = useTranslation()
  const categoryNameById = new Map(categories.map((category) => [category.id, category.name]))
  const categoryName = (id: string) => categoryNameById.get(id) ?? id
  const proposable = (items: LedgerReviewQueueItem[]) => items.filter((item) => reviewProposal(item, llm)).map((item) => item.id)
  const pageIds = proposable(entries.flatMap(entryItems))
  const columnCount = showSelection ? 6 : 5

  function matchWordToggle(ids: string[], word: string) {
    return (
      // oxlint-disable-next-line jsx-a11y/click-events-have-key-events, jsx-a11y/no-noninteractive-element-interactions -- only keeps the checkbox click (mouse or keyboard) from selecting the row
      <label className="flex items-center gap-2 text-xs text-muted-foreground" onClick={stopPropagation}>
        <Checkbox
          checked={checkedState(ids, selection.isMatchWordSelected)}
          onCheckedChange={(checked) => selection.setMatchWord(ids, checked === true)}
        />
        {t("ledger.suggestions.addMatchWord", { word })}
      </label>
    )
  }

  function proposalCell(proposal: ReviewProposal | undefined, matchWordIds: string[]) {
    if (!proposal) return t("ledger.noCategory")
    if (proposal.kind === "assigned") return categoryName(proposal.categoryId)
    return (
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-muted-foreground italic">{categoryName(proposal.categoryId)}</span>
          {proposal.confidence && <LedgerSuggestionBadge confidence={proposal.confidence} />}
        </div>
        {proposal.matchWord && matchWordIds.length > 0 && matchWordToggle(matchWordIds, proposal.matchWord)}
      </div>
    )
  }

  function sourceBadge(item: LedgerReviewQueueItem) {
    return (
      <Badge variant={item.categorizationSource === "keyword" ? "default" : "secondary"}>
        {t(`ledger.categorizationSource.${item.categorizationSource}`)}
      </Badge>
    )
  }

  function transactionRow(item: LedgerReviewQueueItem, inGroup: boolean, override?: string) {
    const proposal = reviewProposal(item, llm)
    const selected = selectedTransactionId === item.id
    return (
      <TableRow
        key={item.id}
        className={cn("cursor-pointer", inGroup && "bg-muted/20", selected && "bg-accent/40")}
        aria-selected={selected}
        onClick={() => onSelect(item)}
      >
        {showSelection && (
          <TableCell onClick={stopPropagation}>
            {proposal && (
              <Checkbox
                checked={selection.isSelected(item.id)}
                onCheckedChange={(checked) => selection.setSelected([item.id], checked === true)}
                aria-label={t("ledger.queue.select")}
              />
            )}
          </TableCell>
        )}
        <TableCell className={cn(inGroup && "pl-8")}>{formatDate(item.bookingDate)}</TableCell>
        <TableCell className={item.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
          {formatMinor(item.amountMinor, item.currency)}
        </TableCell>
        <TableCell>{item.counterpartyName || "-"}</TableCell>
        <TableCell>
          <div className="flex flex-wrap items-center gap-2">
            {override ? categoryName(override) : proposalCell(proposal, proposal?.matchWord ? [item.id] : [])}
            <LedgerSplitBadge transaction={item} categories={categories} />
          </div>
        </TableCell>
        <TableCell>{sourceBadge(item)}</TableCell>
      </TableRow>
    )
  }

  function groupRows(entry: Extract<ReviewEntry, { kind: "group" }>) {
    const { items, proposal, key } = entry
    const ids = items.map((item) => item.id)
    const name = items[0].counterpartyName || "-"
    const expanded = expandedGroups.has(key)
    const override = groupCategory(key)
    const sameCurrency = items.every((item) => item.currency === items[0].currency)
    const total = items.reduce((sum, item) => sum + item.amountMinor, 0)
    const matchWordIds = proposal.matchWord
      ? items.filter((item) => reviewProposal(item, llm)?.matchWord === proposal.matchWord).map((item) => item.id)
      : []
    const dates = [items[items.length - 1].bookingDate, items[0].bookingDate].map((date) => formatDate(date))

    return [
      <TableRow key={key} className="cursor-pointer" onClick={() => onToggleGroup(key)}>
        {showSelection && (
          <TableCell onClick={stopPropagation}>
            <Checkbox
              checked={checkedState(ids, selection.isSelected)}
              onCheckedChange={(checked) => selection.setSelected(ids, checked === true)}
              aria-label={t("ledger.queue.selectGroup", { name })}
            />
          </TableCell>
        )}
        <TableCell className="text-muted-foreground">
          {dates[0] === dates[1] ? dates[0] : `${dates[0]} – ${dates[1]}`}
        </TableCell>
        <TableCell className={total < 0 ? "text-destructive" : "text-emerald-600"}>
          {sameCurrency ? formatMinor(total, items[0].currency) : "-"}
        </TableCell>
        <TableCell>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-6"
              aria-expanded={expanded}
              aria-label={t(expanded ? "ledger.queue.collapseGroup" : "ledger.queue.expandGroup", { name })}
              onClick={(event) => {
                event.stopPropagation()
                onToggleGroup(key)
              }}
            >
              {expanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
            </Button>
            <span className="font-medium">{name}</span>
            <Badge variant="outline">{t("ledger.queue.groupCount", { count: items.length })}</Badge>
          </div>
        </TableCell>
        <TableCell onClick={stopPropagation}>
          <div className="space-y-1">
            <div className="flex items-center gap-1">
              <LedgerCategorySelect
                categories={categories}
                value={override ?? proposal.categoryId}
                onValueChange={(value) => onGroupCategoryChange(key, value === proposal.categoryId ? undefined : value)}
                size="sm"
                className="w-[14rem]"
                aria-label={t("ledger.queue.groupCategory", { name })}
              />
              {override ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="size-7"
                  onClick={() => onGroupCategoryChange(key, undefined)}
                  aria-label={t("ledger.queue.resetGroupCategory", { name: categoryName(proposal.categoryId) })}
                  title={t("ledger.queue.resetGroupCategory", { name: categoryName(proposal.categoryId) })}
                >
                  <Undo2Icon />
                </Button>
              ) : (
                proposal.confidence && <LedgerSuggestionBadge confidence={proposal.confidence} />
              )}
            </div>
            {proposal.matchWord && matchWordIds.length > 0 && matchWordToggle(matchWordIds, proposal.matchWord)}
          </div>
        </TableCell>
        <TableCell>{sourceBadge(items[0])}</TableCell>
      </TableRow>,
      ...(expanded ? items.map((item) => transactionRow(item, true, override)) : []),
    ]
  }

  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            {showSelection && (
              <TableHead className="w-8">
                {pageIds.length > 0 && (
                  <Checkbox
                    checked={checkedState(pageIds, selection.isSelected)}
                    onCheckedChange={(checked) => selection.setSelected(pageIds, checked === true)}
                    aria-label={t("ledger.queue.selectPage")}
                  />
                )}
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
          {entries.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-8 text-center text-muted-foreground">
                {t("ledger.noTransactionsToReview")}
              </TableCell>
            </TableRow>
          ) : (
            entries.flatMap((entry) => (entry.kind === "group" ? groupRows(entry) : [transactionRow(entry.item, false)]))
          )}
        </TableBody>
      </Table>
    </div>
  )
}
