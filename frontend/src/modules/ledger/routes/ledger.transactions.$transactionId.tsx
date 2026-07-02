import { getRouteApi } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerTransaction } from "@/modules/ledger/hooks/use-ledger"
import { LedgerTransactionDetailsCard } from "@/modules/ledger/components/ledger-transaction-details-card"

const routeApi = getRouteApi("/ledger/transactions/$transactionId")

export function LedgerTransactionPage() {
  const { t } = useTranslation()
  const { transactionId } = routeApi.useParams()
  const { data: transaction } = useLedgerTransaction(transactionId)

  usePageTitle(t("ledger.transaction"), t("app.title"))

  if (!transaction) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }

  return <LedgerTransactionDetailsCard transaction={transaction} />
}
