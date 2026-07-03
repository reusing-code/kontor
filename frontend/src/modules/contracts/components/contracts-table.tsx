import { Fragment, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { format } from "date-fns"
import { ChevronRight, ExternalLink, FileText, MoreVertical } from "lucide-react"
import type { Contract } from "@/modules/contracts/types"
import { contractFields } from "@/modules/contracts/config/contract-fields"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { cn } from "@/lib/utils"

const tableColumns = contractFields
  .filter((f) => f.showInTable)
  .sort((a, b) => a.tableOrder - b.tableOrder)

const detailFields = contractFields.filter((f) => !f.showInTable)

interface ContractsTableProps {
  contracts: Contract[]
  onEdit: (contract: Contract) => void
  onDelete: (contract: Contract) => void
  getRowClassName?: (contract: Contract) => string | undefined
}

function formatCellValue(contract: Contract, key: string, currency: string, t: (key: string) => string): string {
  const value = contract[key as keyof Contract]
  if (value === undefined || value === null || value === "") return "-"
  if (key === "price") {
    const interval = contract.billingInterval === "yearly" ? t("common.perYear") : t("common.perMonth")
    return `${Number(value).toFixed(2)} ${currency} ${interval}`
  }
  if (key === "startDate" || key === "endDate") return format(new Date(value as string), "yyyy-MM-dd")
  if (key === "minimumDurationMonths" || key === "extensionDurationMonths" || key === "noticePeriodMonths") {
    return `${value} ${t("common.months")}`
  }
  return String(value)
}

interface ContractDetailRowProps {
  contract: Contract
  colSpan: number
}

function ContractDetailRow({ contract, colSpan }: ContractDetailRowProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")

  return (
    <TableRow className="bg-muted/30 hover:bg-muted/30">
      <TableCell colSpan={colSpan} className="p-0">
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4 p-4">
            {detailFields.map((field) => {
              const value = formatCellValue(contract, field.key, currency, t)
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

export function ContractsTable({ contracts, onEdit, onDelete, getRowClassName }: ContractsTableProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const [expandedId, setExpandedId] = useState<string | null>(null)

  const totalColumns = tableColumns.length + 4 // +1 chevron, +1 cancellation, +1 links, +1 actions

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
            <TableHead>{t("contract.cancellationDate")}</TableHead>
            <TableHead>{t("contract.links")}</TableHead>
            <TableHead className="w-12" />
          </TableRow>
        </TableHeader>
        <TableBody>
          {contracts.length === 0 ? (
            <TableRow>
              <TableCell colSpan={totalColumns} className="text-center text-muted-foreground py-8">
                {t("contract.noContracts")}
              </TableCell>
            </TableRow>
          ) : (
            contracts.map((contract) => {
              const rowClass = getRowClassName?.(contract)
              const isExpanded = expandedId === contract.id
              return (
                <Fragment key={contract.id}>
                    <TableRow
                      className={cn(
                        contract.expired ? "opacity-50" : undefined,
                        rowClass,
                        "cursor-pointer select-none"
                      )}
                      onClick={() => toggleExpand(contract.id)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault()
                          toggleExpand(contract.id)
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
                           {col.key === "name" ? (
                             <Link to="/contracts/$contractId" params={{ contractId: contract.id }} className="text-primary hover:underline" onClick={(e) => e.stopPropagation()}>
                               {formatCellValue(contract, col.key, currency, t)}
                             </Link>
                           ) : (
                             formatCellValue(contract, col.key, currency, t)
                           )}
                         </TableCell>
                       ))}
                      <TableCell>
                        {contract.expired ? (
                          <Badge variant="secondary">{t("contract.expired")}</Badge>
                        ) : contract.cancellationDate ? (
                          contract.cancellationDate
                        ) : (
                          "-"
                        )}
                      </TableCell>
                      <TableCell onClick={(e) => e.stopPropagation()}>
                        <div className="flex gap-1">
                          {contract.customerPortalUrl && (
                            <a href={contract.customerPortalUrl} target="_blank" rel="noopener noreferrer">
                              <Button variant="ghost" size="icon" className="h-8 w-8" title={t("fields.customerPortalUrl")}>
                                <ExternalLink className="h-4 w-4" />
                              </Button>
                            </a>
                          )}
                          {contract.paperlessUrl && (
                            <a href={contract.paperlessUrl} target="_blank" rel="noopener noreferrer">
                              <Button variant="ghost" size="icon" className="h-8 w-8" title={t("fields.paperlessUrl")}>
                                <FileText className="h-4 w-4" />
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
                            <DropdownMenuItem onClick={() => onEdit(contract)}>
                              {t("common.edit")}
                            </DropdownMenuItem>
                            <DropdownMenuItem onClick={() => onDelete(contract)} className="text-destructive">
                              {t("common.delete")}
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </TableCell>
                    </TableRow>
                    {isExpanded && <ContractDetailRow contract={contract} colSpan={totalColumns} />}
                </Fragment>
              )
            })
          )}
        </TableBody>
      </Table>
    </div>
  )
}
