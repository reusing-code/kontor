import { Fragment, useState } from "react"
import { useTranslation } from "react-i18next"
import { format } from "date-fns"
import { ChevronRight, MoreVertical } from "lucide-react"
import type { CostEntry } from "@/modules/auto/types"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

interface CostEntriesTableProps {
  entries: CostEntry[]
  onEdit: (entry: CostEntry) => void
  onDelete: (entry: CostEntry) => void
}

function CostEntryDetailRow({ entry, colSpan }: { entry: CostEntry; colSpan: number }) {
  const { t } = useTranslation()

  return (
    <TableRow className="bg-muted/30 hover:bg-muted/30">
      <TableCell colSpan={colSpan} className="p-0">
        <div className="grid grid-cols-2 md:grid-cols-3 gap-4 p-4">
          {entry.vendor && (
            <div>
              <div className="text-xs text-muted-foreground">{t("costEntryFields.vendor")}</div>
              <div className="text-sm">{entry.vendor}</div>
            </div>
          )}
          {entry.comments && (
            <div className="col-span-full">
              <div className="text-xs text-muted-foreground">{t("costEntryFields.comments")}</div>
              <div className="text-sm whitespace-pre-wrap">{entry.comments}</div>
            </div>
          )}
        </div>
      </TableCell>
    </TableRow>
  )
}

export function CostEntriesTable({ entries, onEdit, onDelete }: CostEntriesTableProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const totalColumns = 7 // chevron + date + type + description + amount + mileage + actions

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id))
  }

  const sorted = [...entries].sort((a, b) => b.date.localeCompare(a.date))

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10" />
            <TableHead>{t("costEntryFields.date")}</TableHead>
            <TableHead>{t("costEntryFields.type")}</TableHead>
            <TableHead>{t("costEntryFields.description")}</TableHead>
            <TableHead className="text-right">{t("costEntryFields.amount")}</TableHead>
            <TableHead className="text-right">{t("costEntryFields.mileage")}</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {sorted.length === 0 ? (
            <TableRow>
              <TableCell colSpan={totalColumns} className="text-center text-muted-foreground py-8">
                {t("costEntry.noCostEntries")}
              </TableCell>
            </TableRow>
          ) : (
            sorted.map((entry) => {
              const isExpanded = expandedId === entry.id
              const hasDetails = !!(entry.vendor || entry.comments)
              return (
                <Fragment key={entry.id}>
                  <TableRow
                    className={cn("select-none", hasDetails && "cursor-pointer")}
                    onClick={() => hasDetails && toggleExpand(entry.id)}
                    onKeyDown={(e) => {
                      if (hasDetails && (e.key === "Enter" || e.key === " ")) {
                        e.preventDefault()
                        toggleExpand(entry.id)
                      }
                    }}
                    tabIndex={hasDetails ? 0 : undefined}
                  >
                    <TableCell className="w-10">
                      {hasDetails && (
                        <ChevronRight
                          className={cn(
                            "h-4 w-4 transition-transform duration-200",
                            isExpanded && "rotate-90"
                          )}
                        />
                      )}
                    </TableCell>
                    <TableCell>{format(new Date(entry.date), "yyyy-MM-dd")}</TableCell>
                    <TableCell>{t(`costTypes.${entry.type}`)}</TableCell>
                    <TableCell>{entry.description || "-"}</TableCell>
                    <TableCell className="text-right">
                      {entry.amount != null ? `${entry.amount.toFixed(2)} ${currency}` : "-"}
                    </TableCell>
                    <TableCell className="text-right">
                      {entry.mileage != null ? `${entry.mileage.toLocaleString()} km` : "-"}
                    </TableCell>
                    <TableCell onClick={(e) => e.stopPropagation()}>
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="icon" className="h-8 w-8">
                            <MoreVertical className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onClick={() => onEdit(entry)}>
                            {t("common.edit")}
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => onDelete(entry)} className="text-destructive">
                            {t("common.delete")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                  {isExpanded && hasDetails && (
                    <CostEntryDetailRow entry={entry} colSpan={totalColumns} />
                  )}
                </Fragment>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
