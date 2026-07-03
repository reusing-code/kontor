import { Fragment, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { format } from "date-fns"
import { ChevronRight, ExternalLink, FileText, BookOpen, MoreVertical } from "lucide-react"
import type { Purchase } from "@/modules/purchases/types"
import { purchaseFields } from "@/modules/purchases/config/purchase-fields"
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

const tableColumns = purchaseFields
  .filter((f) => f.showInTable)
  .sort((a, b) => a.tableOrder - b.tableOrder)

const detailFields = purchaseFields.filter((f) => !f.showInTable)

interface PurchasesTableProps {
  purchases: Purchase[]
  onEdit: (purchase: Purchase) => void
  onDelete: (purchase: Purchase) => void
}

function formatCellValue(purchase: Purchase, key: string, currency: string): string {
  const value = purchase[key as keyof Purchase]
  if (value === undefined || value === null || value === "") return "-"
  if (key === "price") return `${Number(value).toFixed(2)} ${currency}`
  if (key === "purchaseDate") return format(new Date(value as string), "yyyy-MM-dd")
  return String(value)
}

interface PurchaseDetailRowProps {
  purchase: Purchase
  colSpan: number
}

function PurchaseDetailRow({ purchase, colSpan }: PurchaseDetailRowProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")

  return (
    <TableRow className="bg-muted/30 hover:bg-muted/30">
      <TableCell colSpan={colSpan} className="p-0">
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4 p-4">
            {detailFields.map((field) => {
              const value = formatCellValue(purchase, field.key, currency)
              if (value === "-" && field.type === "textarea") return null
              return (
                <div key={field.key} className={cn(field.type === "textarea" ? "col-span-full" : "")}>
                  <div className="text-xs text-muted-foreground">{t(field.i18nKey)}</div>
                  <div className={cn("text-sm", field.type === "textarea" && "whitespace-pre-wrap")}>
                    {field.type === "url" && value !== "-" ? (
                      <a
                        href={value}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-primary hover:underline break-all"
                      >
                        {value}
                      </a>
                    ) : (
                      value
                    )}
                  </div>
                </div>
              )
            })}
          </div>
      </TableCell>
    </TableRow>
  )
}

export function PurchasesTable({ purchases, onEdit, onDelete }: PurchasesTableProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const totalColumns = tableColumns.length + 3 // +1 chevron, +1 links, +1 actions

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id))
  }

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-10" />
            {tableColumns.map((col) => (
              <TableHead key={col.key}>{t(col.i18nKey)}</TableHead>
            ))}
            <TableHead>{t("purchase.links")}</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {purchases.length === 0 ? (
            <TableRow>
              <TableCell colSpan={totalColumns} className="text-center text-muted-foreground py-8">
                {t("purchase.noPurchases")}
              </TableCell>
            </TableRow>
          ) : (
            purchases.map((purchase) => {
              const isExpanded = expandedId === purchase.id
              return (
                <Fragment key={purchase.id}>
                    <TableRow
                      className="cursor-pointer select-none"
                      onClick={() => toggleExpand(purchase.id)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault()
                          toggleExpand(purchase.id)
                        }
                      }}
                      tabIndex={0}
                    >
                      <TableCell className="w-10">
                        <ChevronRight
                          className={cn(
                            "h-4 w-4 transition-transform duration-200",
                            isExpanded && "rotate-90"
                          )}
                        />
                      </TableCell>
                       {tableColumns.map((col) => (
                         <TableCell key={col.key}>
                           {col.key === "itemName" ? (
                             <Link to="/purchases/$purchaseId" params={{ purchaseId: purchase.id }} className="text-primary hover:underline" onClick={(e) => e.stopPropagation()}>
                               {formatCellValue(purchase, col.key, currency)}
                             </Link>
                           ) : (
                             formatCellValue(purchase, col.key, currency)
                           )}
                         </TableCell>
                       ))}
                      <TableCell onClick={(e) => e.stopPropagation()}>
                        <div className="flex gap-1">
                          {purchase.descriptionUrl && (
                            <a href={purchase.descriptionUrl} target="_blank" rel="noopener noreferrer">
                              <Button variant="ghost" size="icon" className="h-8 w-8" title={t("purchaseFields.descriptionUrl")}>
                                <ExternalLink className="h-4 w-4" />
                              </Button>
                            </a>
                          )}
                          {purchase.invoiceUrl && (
                            <a href={purchase.invoiceUrl} target="_blank" rel="noopener noreferrer">
                              <Button variant="ghost" size="icon" className="h-8 w-8" title={t("purchaseFields.invoiceUrl")}>
                                <FileText className="h-4 w-4" />
                              </Button>
                            </a>
                          )}
                          {purchase.handbookUrl && (
                            <a href={purchase.handbookUrl} target="_blank" rel="noopener noreferrer">
                              <Button variant="ghost" size="icon" className="h-8 w-8" title={t("purchaseFields.handbookUrl")}>
                                <BookOpen className="h-4 w-4" />
                              </Button>
                            </a>
                          )}
                        </div>
                      </TableCell>
                      <TableCell onClick={(e) => e.stopPropagation()}>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button variant="ghost" size="icon" className="h-8 w-8">
                              <MoreVertical className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent align="end">
                            <DropdownMenuItem onClick={() => onEdit(purchase)}>
                              {t("common.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => onDelete(purchase)} className="text-destructive">
                              {t("common.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                    {isExpanded && <PurchaseDetailRow purchase={purchase} colSpan={totalColumns} />}
                </Fragment>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
