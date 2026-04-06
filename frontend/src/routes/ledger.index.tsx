import { createRoute, Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerAccounts, useLedgerCategories, useLedgerImports, useLedgerReviewQueue } from "@/hooks/use-ledger"
import { LedgerAccountList } from "@/components/ledger-account-list"
import { LedgerImportPanel } from "@/components/ledger-import-panel"
import { LedgerImportsList } from "@/components/ledger-imports-list"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { rootRoute } from "./__root"

export const ledgerIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger",
  component: LedgerIndexPage,
})

function LedgerIndexPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.ledger"), t("app.title"))

  const { data: accounts = [] } = useLedgerAccounts()
  const { data: imports = [] } = useLedgerImports()
  const { data: categories = [] } = useLedgerCategories()
  const { data: reviewPage } = useLedgerReviewQueue(10)

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("nav.ledger")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.description")}</p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <CardTitle>{t("ledger.reviewQueue")}</CardTitle>
            <Button asChild variant="outline">
              <Link to="/ledger/review">{t("ledger.openReview")}</Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-semibold">{reviewPage?.items.length ?? 0}</div>
            <p className="text-sm text-muted-foreground">{t("ledger.transactionsNeedReview")}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-4">
            <CardTitle>{t("ledger.categories")}</CardTitle>
            <Button asChild variant="outline">
              <Link to="/ledger/categories">{t("ledger.manageCategories")}</Link>
            </Button>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-semibold">{categories.length}</div>
            <p className="text-sm text-muted-foreground">{t("ledger.categoryTreeDescription")}</p>
          </CardContent>
        </Card>
      </div>

      <LedgerImportPanel accounts={accounts} />
      <LedgerAccountList accounts={accounts} />
      <LedgerImportsList imports={imports} accounts={accounts} />
    </div>
  )
}
