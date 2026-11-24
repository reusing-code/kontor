import { useCallback } from "react"
import { getRouteApi, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { LedgerAllTransactions } from "@/modules/ledger/components/ledger-all-transactions"
import { allTransactionsSearch } from "@/modules/ledger/lib/transaction-filter"

const routeApi = getRouteApi("/ledger/transactions")

export function LedgerAllTransactionsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { accounts, page = 1 } = routeApi.useSearch()

  usePageTitle(t("ledger.allTransactions.title"), t("app.title"))

  const changePage = useCallback(
    (next: number, options?: { replace?: boolean }) =>
      void navigate({ to: "/ledger/transactions", search: allTransactionsSearch(accounts, next), replace: options?.replace }),
    [navigate, accounts],
  )

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.allTransactions.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.allTransactions.description")}</p>
      </div>
      <LedgerAllTransactions
        accountIds={accounts}
        // another selection starts again at the first page
        onAccountIdsChange={(ids) => void navigate({ to: "/ledger/transactions", search: allTransactionsSearch(ids, 1), replace: true })}
        page={page}
        onPageChange={changePage}
      />
    </div>
  )
}
