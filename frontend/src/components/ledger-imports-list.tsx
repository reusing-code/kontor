import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import type { LedgerAccount, LedgerImportBatch } from "@/types/ledger"
import { formatLedgerDate, formatSourceType } from "@/lib/ledger-utils"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface LedgerImportsListProps {
  imports: LedgerImportBatch[]
  accounts: LedgerAccount[]
}

export function LedgerImportsList({ imports, accounts }: LedgerImportsListProps) {
  const { t } = useTranslation()
  const accountNameById = useMemo(
    () => new Map(accounts.map((account) => [account.id, account.name])),
    [accounts],
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.importHistory")}</CardTitle>
      </CardHeader>
      <CardContent>
        {imports.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("ledger.noImports")}</p>
        ) : (
          <div className="overflow-x-auto rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("ledger.date")}</TableHead>
                  <TableHead>{t("ledger.sourceType")}</TableHead>
                  <TableHead>{t("ledger.file")}</TableHead>
                  <TableHead>{t("ledger.account")}</TableHead>
                  <TableHead>{t("ledger.importedRows")}</TableHead>
                  <TableHead>{t("ledger.duplicateRows")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {imports.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell>{formatLedgerDate(item.createdAt)}</TableCell>
                    <TableCell>{formatSourceType(item.sourceType)}</TableCell>
                    <TableCell>{item.filename}</TableCell>
                    <TableCell>{accountNameById.get(item.accountId) ?? item.accountId}</TableCell>
                    <TableCell>{item.importedRows}</TableCell>
                    <TableCell>{item.duplicateRows}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
