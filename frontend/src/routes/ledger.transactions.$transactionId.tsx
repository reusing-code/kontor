import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerTransaction } from "@/hooks/use-ledger"
import { LedgerTransactionDetailsCard } from "@/components/ledger-transaction-details-card"
import { rootRoute } from "./__root"

export const ledgerTransactionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/transactions/$transactionId",
  component: LedgerTransactionPage,
})

export function LedgerTransactionPage() {
  const { t } = useTranslation()
  const { transactionId } = ledgerTransactionRoute.useParams()
  const { data: transaction } = useLedgerTransaction(transactionId)

  usePageTitle(t("ledger.transaction"), t("app.title"))

  if (!transaction) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }

  return <LedgerTransactionDetailsCard transaction={transaction} />
}
