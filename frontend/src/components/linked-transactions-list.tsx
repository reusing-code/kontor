import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useModules } from "@/hooks/use-modules"
import { useLedgerTransaction } from "@/modules/ledger/hooks/use-ledger"
import { formatAmountMinor, formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import { transactionPath } from "@/modules/ledger/lib/module-links"

interface LinkedTransactionsListProps {
  transactionIds: string[]
}

function LinkedTransactionRow({ transactionId }: { transactionId: string }) {
  const { i18n, t } = useTranslation()
  const { data: transaction } = useLedgerTransaction(transactionId)

  if (!transaction) {
    return <div className="text-sm text-muted-foreground">{transactionId}</div>
  }

  return (
    <Link to={transactionPath(transaction.id)} className="block rounded-md border p-3 text-sm transition-colors hover:bg-accent/40">
      <div className="flex items-center justify-between gap-3">
        <span>{formatLedgerDate(transaction.bookingDate)}</span>
        <span className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
          {formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}
        </span>
      </div>
      <div className="mt-1 text-muted-foreground">{transaction.counterpartyName || transaction.purpose || t("ledger.transaction")}</div>
    </Link>
  )
}

export function LinkedTransactionsList({ transactionIds }: LinkedTransactionsListProps) {
  const { t } = useTranslation()
  const { isEnabled } = useModules()

  if (!isEnabled("ledger")) {
    return null
  }

  if (transactionIds.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("ledger.noLinkedTransactions")}</p>
  }

  return (
    <div className="space-y-2">
      {transactionIds.map((transactionId) => (
        <LinkedTransactionRow key={transactionId} transactionId={transactionId} />
      ))}
    </div>
  )
}
