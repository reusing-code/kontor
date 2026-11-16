import { useTranslation } from "react-i18next"
import type { DraftTargetType } from "@/types/drafts"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

export type DraftTypeFilter = DraftTargetType | "all"

interface BulkAcceptBarProps {
  // Types present in the inbox with their number of drafts, in display order.
  typeCounts: [DraftTargetType, number][]
  total: number
  filter: DraftTypeFilter
  onFilterChange: (filter: DraftTypeFilter) => void
  // Shown drafts that can be selected, and how many of them are.
  selectableCount: number
  selectedCount: number
  onSelectAll: (selected: boolean) => void
  onAccept: () => void
  // Set while a bulk accept is running.
  progress: { done: number; total: number } | null
}

// BulkAcceptBar filters the inbox by type and accepts the selected drafts.
export function BulkAcceptBar({
  typeCounts,
  total,
  filter,
  onFilterChange,
  selectableCount,
  selectedCount,
  onSelectAll,
  onAccept,
  progress,
}: BulkAcceptBarProps) {
  const { t } = useTranslation()
  const running = progress !== null
  const selectAllId = "drafts-select-all"

  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border p-3">
      <div className="flex items-center gap-2">
        <Checkbox
          id={selectAllId}
          checked={selectableCount > 0 && selectedCount === selectableCount}
          disabled={running || selectableCount === 0}
          onCheckedChange={(checked) => onSelectAll(checked === true)}
        />
        <label htmlFor={selectAllId} className="text-sm">
          {t("drafts.bulk.selectAll", { count: selectableCount })}
        </label>
      </div>

      {typeCounts.length > 1 && (
        <Select value={filter} onValueChange={(value) => onFilterChange(value as DraftTypeFilter)} disabled={running}>
          <SelectTrigger className="w-auto" aria-label={t("drafts.bulk.filterType")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("drafts.bulk.allTypes", { count: total })}</SelectItem>
            {typeCounts.map(([type, count]) => (
              <SelectItem key={type} value={type}>
                {t("drafts.bulk.typeCount", { type: t(`drafts.types.${type}`), count })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}

      <div className="ml-auto flex items-center gap-3">
        <span className="text-sm text-muted-foreground" aria-live="polite">
          {running
            ? t("drafts.bulk.progress", { done: progress.done, total: progress.total })
            : selectedCount > 0 && t("drafts.bulk.selected", { count: selectedCount })}
        </span>
        <Button onClick={onAccept} disabled={running || selectedCount === 0}>
          {t("drafts.bulk.accept")}
        </Button>
      </div>
    </div>
  )
}
