import { getRouteApi, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { LedgerAllTransactions } from "@/modules/ledger/components/ledger-all-transactions"

const routeApi = getRouteApi("/ledger/transactions")

export function LedgerAllTransactionsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { accounts } = routeApi.useSearch()

  usePageTitle(t("ledger.allTransactions.title"), t("app.title"))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.allTransactions.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.allTransactions.description")}</p>
      </div>
      <LedgerAllTransactions
        accountIds={accounts}
        onAccountIdsChange={(ids) => void navigate({ to: "/ledger/transactions", search: ids.length > 0 ? { accounts: ids } : {}, replace: true })}
      />
    </div>
  )
}
