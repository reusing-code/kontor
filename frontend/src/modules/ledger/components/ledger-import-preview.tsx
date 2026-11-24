import { formatDate, formatMinor } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { formatSourceType } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerPreviewResult } from "@/modules/ledger/types"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

export function LedgerImportPreviewSummary({ preview, accountId, accountName }: {
  preview: LedgerPreviewResult
  accountId: string
  accountName: string | undefined
}) {
  const { t } = useTranslation()

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{formatSourceType(preview.sourceType)}</Badge>
        {preview.bankName && <Badge variant="outline">{preview.bankName}</Badge>}
        {preview.iban && <Badge variant="outline">{preview.iban}</Badge>}
        {accountId && (
          <Badge variant="outline">{accountName ?? t("ledger.accountResolved")}</Badge>
        )}
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <div>
          <div className="text-sm text-muted-foreground">{t("ledger.totalRows")}</div>
          <div className="text-2xl font-semibold">{preview.totalRows}</div>
        </div>
        <div>
          <div className="text-sm text-muted-foreground">{t("ledger.newRows")}</div>
          <div className="text-2xl font-semibold">{preview.newRows}</div>
        </div>
        <div>
          <div className="text-sm text-muted-foreground">{t("ledger.duplicateRows")}</div>
          <div className="text-2xl font-semibold">{preview.duplicateRows}</div>
        </div>
      </div>

      {preview.balance && (
        <div className="text-sm text-muted-foreground">
          {t("ledger.balance.inFile", { amount: formatMinor(preview.balance.amountMinor), date: preview.balance.asOf })}
        </div>
      )}

      {preview.warnings && preview.warnings.length > 0 && (
        <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm">
          <div className="font-medium">{t("ledger.warnings")}</div>
          <ul className="mt-2 space-y-1 text-muted-foreground">
            {preview.warnings.map((warning) => (
              <li key={warning}>{warning}</li>
            ))}
          </ul>
        </div>
      )}
    </>
  )
}

export function LedgerImportPreviewTable({ transactions }: { transactions: LedgerPreviewResult["transactions"] }) {
  const { t } = useTranslation()

  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("ledger.date")}</TableHead>
            <TableHead>{t("ledger.valueDate")}</TableHead>
            <TableHead>{t("ledger.amount")}</TableHead>
            <TableHead>{t("ledger.counterparty")}</TableHead>
            <TableHead>{t("ledger.purpose")}</TableHead>
            <TableHead>{t("ledger.category")}</TableHead>
            <TableHead>{t("ledger.status")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {transactions.map((txn) => (
            <TableRow key={txn.fingerprint}>
              <TableCell>{formatDate(txn.row.bookingDate)}</TableCell>
              <TableCell>{txn.row.valueDate ? formatDate(txn.row.valueDate) : "-"}</TableCell>
              <TableCell className={txn.row.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                {formatMinor(txn.row.amountMinor, txn.row.currency)}
              </TableCell>
              <TableCell>{txn.row.counterpartyName || "-"}</TableCell>
              <TableCell className="max-w-[24rem] truncate">{txn.row.purpose || "-"}</TableCell>
              <TableCell>{txn.suggestedCategoryName || t("ledger.noCategory")}</TableCell>
              <TableCell>
                <div className="flex flex-wrap gap-2">
                  <Badge variant={txn.isDuplicate ? "secondary" : "default"}>
                    {txn.isDuplicate ? t("ledger.duplicate") : t("ledger.new")}
                  </Badge>
                  <Badge variant="outline">{t(`ledger.categorizationSource.${txn.categorizationSource}`)}</Badge>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
