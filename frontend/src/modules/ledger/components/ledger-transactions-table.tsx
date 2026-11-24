import { Link } from "@tanstack/react-router"
import { formatDate, formatMinor } from "@/lib/format"
import { useTranslation } from "react-i18next"
import type { LedgerAccount, LedgerCategory, LedgerPageBalance, LedgerTransaction } from "@/modules/ledger/types"
import { transactionPath } from "@/modules/ledger/lib/module-links"
import { formatLedgerCategorizationSource, formatLedgerReviewStatus, formatLedgerSpecialCategory, formatSourceType } from "@/modules/ledger/lib/ledger-utils"
import { LedgerSplitBadge } from "@/modules/ledger/components/ledger-split-badge"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface LedgerTransactionsTableProps {
  transactions: LedgerTransaction[]
  categories?: LedgerCategory[]
  // the balances around the shown page; no balance rows without it
  pageBalance?: LedgerPageBalance
  currency?: string
  // shows an account column (for lists across accounts)
  accounts?: LedgerAccount[]
}

export function LedgerTransactionsTable({ transactions, categories = [], pageBalance, currency = "EUR", accounts }: LedgerTransactionsTableProps) {
  const { t } = useTranslation()
  const categoryNameById = new Map(categories.map((category) => [category.id, category.name]))
  const accountNameById = accounts ? new Map(accounts.map((account) => [account.id, account.name])) : undefined
  const columnCount = accountNameById ? 9 : 8
  const columnsBeforeAmount = accountNameById ? 3 : 2
  const showBalance = pageBalance !== undefined && transactions.length > 0

  function balanceRow(label: string, minor: number) {
    return (
      <TableRow className="bg-muted/50 font-medium hover:bg-muted/50">
        <TableCell colSpan={columnsBeforeAmount}>{label}</TableCell>
        <TableCell className={minor < 0 ? "text-destructive" : undefined}>{formatMinor(minor, currency)}</TableCell>
        <TableCell colSpan={columnCount - columnsBeforeAmount - 1} />
      </TableRow>
    )
  }

  return (
    <div className="overflow-x-auto rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("ledger.date")}</TableHead>
            <TableHead>{t("ledger.valueDate")}</TableHead>
            {accountNameById && <TableHead>{t("ledger.account")}</TableHead>}
            <TableHead>{t("ledger.amount")}</TableHead>
            <TableHead>{t("ledger.counterparty")}</TableHead>
            <TableHead>{t("ledger.purpose")}</TableHead>
            <TableHead>{t("ledger.category")}</TableHead>
            <TableHead>{t("ledger.reviewState")}</TableHead>
            <TableHead>{t("ledger.source")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {transactions.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columnCount} className="py-8 text-center text-muted-foreground">
                {t("ledger.noTransactions")}
              </TableCell>
            </TableRow>
          ) : (
            <>
              {showBalance && balanceRow(t("ledger.balance.afterPage"), pageBalance.closingMinor)}
              {transactions.map((txn) => (
                <TableRow key={txn.id}>
                  <TableCell>
                    <Link to={transactionPath(txn.id)} className="text-primary hover:underline">
                      {formatDate(txn.bookingDate)}
                    </Link>
                  </TableCell>
                  <TableCell>{txn.valueDate ? formatDate(txn.valueDate) : "-"}</TableCell>
                  {accountNameById && (
                    <TableCell>
                      <Link to="/ledger/accounts/$accountId" params={{ accountId: txn.accountId }} className="hover:underline">
                        {accountNameById.get(txn.accountId) ?? "-"}
                      </Link>
                    </TableCell>
                  )}
                  <TableCell className={txn.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                    {formatMinor(txn.amountMinor, txn.currency)}
                  </TableCell>
                  <TableCell>{txn.counterpartyName || "-"}</TableCell>
                  <TableCell className="max-w-[24rem] truncate">{txn.purpose || "-"}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap items-center gap-2">
                      {txn.categoryId ? categoryNameById.get(txn.categoryId) ?? txn.categoryId : t("ledger.noCategory")}
                      <LedgerSplitBadge transaction={txn} categories={categories} />
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-2">
                      {txn.specialCategory ? (
                        <Badge variant="secondary">{formatLedgerSpecialCategory(txn.specialCategory)}</Badge>
                      ) : null}
                      <Badge variant={txn.reviewStatus === "confirmed" ? "secondary" : "default"}>
                        {formatLedgerReviewStatus(txn.reviewStatus)}
                      </Badge>
                      <Badge variant="outline">{formatLedgerCategorizationSource(txn.categorizationSource)}</Badge>
                    </div>
                  </TableCell>
                  <TableCell>{formatSourceType(txn.sourceType)}</TableCell>
                </TableRow>
              ))}
              {showBalance && balanceRow(t("ledger.balance.beforePage"), pageBalance.openingMinor)}
            </>
          )}
        </TableBody>
      </Table>
    </div>
  )
}
