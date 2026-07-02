import { useState } from "react"
import { getRouteApi } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerAccount, useLedgerCategories, useLedgerTransactions } from "@/modules/ledger/hooks/use-ledger"
import { formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

const routeApi = getRouteApi("/ledger/accounts/$accountId")

export function LedgerAccountPage() {
  const { t } = useTranslation()
  const { accountId } = routeApi.useParams()
  const { data: account } = useLedgerAccount(accountId)
  const { data: categories = [] } = useLedgerCategories()
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const cursor = cursorStack.length > 0 ? cursorStack[cursorStack.length - 1] : undefined
  const { data: page, isFetching } = useLedgerTransactions(accountId, 100, cursor)

  usePageTitle(account?.name ?? t("nav.ledger"), t("app.title"))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{account?.name ?? "..."}</h1>
        <p className="text-sm text-muted-foreground">{account?.bank}</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.accountDetails")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-4">
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.bank")}</div>
            <div className="text-sm">{account?.bank ?? "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.iban")}</div>
            <div className="text-sm">{account?.iban ?? t("ledger.noIban")}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.currency")}</div>
            <div className="text-sm">{account?.currency ?? "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.createdAt")}</div>
            <div className="text-sm">{formatLedgerDate(account?.createdAt)}</div>
          </div>
        </CardContent>
      </Card>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("ledger.transactions")}</h2>
        <LedgerTransactionsTable
          transactions={page?.items ?? []}
          categories={categories}
          nextCursor={page?.nextCursor}
          loadingMore={isFetching}
          onLoadMore={page?.nextCursor ? () => setCursorStack((prev) => [...prev, page.nextCursor as string]) : undefined}
        />
      </div>
    </div>
  )
}
