import { createRoute, Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useQuery } from "@tanstack/react-query"
import { rootRoute } from "./__root"
import { getSummary } from "@/lib/contract-repository"
import { getPurchaseSummary } from "@/lib/purchase-repository"
import { useLedgerAccounts, useLedgerImports, useLedgerReviewQueue } from "@/hooks/use-ledger"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { FileText, ShoppingBag, ArrowRight, CalendarClock, Landmark } from "lucide-react"

export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const { t } = useTranslation()
  usePageTitle(t("home.title"), t("app.title"))

  const { data: contractSummary } = useQuery({
    queryKey: ["summary"],
    queryFn: getSummary,
  })

  const { data: purchaseSummary } = useQuery({
    queryKey: ["purchases-summary"],
    queryFn: getPurchaseSummary,
  })

  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const { data: ledgerImports = [] } = useLedgerImports()
  const { data: ledgerReviewPage } = useLedgerReviewQueue(10)

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">{t("home.title")}</h1>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <Link to="/contracts" className="group">
          <Card className="transition-colors group-hover:bg-accent/50">
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                    <FileText className="h-5 w-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">{t("nav.contracts")}</CardTitle>
                </div>
                <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-2xl font-bold">{contractSummary?.totalContracts ?? 0}</p>
                  <p className="text-sm text-muted-foreground">{t("home.activeContracts")}</p>
                </div>
                <div>
                  <p className="text-2xl font-bold">
                    {(contractSummary?.totalMonthlyAmount ?? 0).toFixed(2)} {t("common.currency")}
                  </p>
                  <p className="text-sm text-muted-foreground">{t("home.monthlySpend")}</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </Link>

        <Link to="/purchases" className="group">
          <Card className="transition-colors group-hover:bg-accent/50">
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                    <ShoppingBag className="h-5 w-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">{t("nav.purchases")}</CardTitle>
                </div>
                <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-2xl font-bold">{purchaseSummary?.totalPurchases ?? 0}</p>
                  <p className="text-sm text-muted-foreground">{t("home.totalPurchases")}</p>
                </div>
                <div>
                  <p className="text-2xl font-bold">
                    {(purchaseSummary?.totalSpent ?? 0).toFixed(2)} {t("common.currency")}
                  </p>
                  <p className="text-sm text-muted-foreground">{t("home.totalSpent")}</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </Link>

        <Link to="/ledger" className="group">
          <Card className="transition-colors group-hover:bg-accent/50">
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                    <Landmark className="h-5 w-5 text-primary" />
                  </div>
                  <CardTitle className="text-xl">{t("nav.ledger")}</CardTitle>
                </div>
                <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
              </div>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-2xl font-bold">{ledgerAccounts.length}</p>
                  <p className="text-sm text-muted-foreground">{t("ledger.accounts")}</p>
                </div>
                <div>
                  <p className="text-2xl font-bold">{ledgerImports.length}</p>
                  <p className="text-sm text-muted-foreground">{t("ledger.importHistory")}</p>
                </div>
                <div>
                  <p className="text-2xl font-bold">{ledgerReviewPage?.items.length ?? 0}</p>
                  <p className="text-sm text-muted-foreground">{t("ledger.reviewQueue")}</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </Link>
      </div>

      <div className="grid gap-6 sm:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CalendarClock className="h-4 w-4" />
              {t("home.quickStats")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between">
                <dt className="text-muted-foreground">{t("home.yearlyContracts")}</dt>
                <dd className="font-medium">
                  {(contractSummary?.totalYearlyAmount ?? 0).toFixed(2)} {t("common.currency")}
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-muted-foreground">{t("home.categories")}</dt>
                <dd className="font-medium">
                  {(contractSummary?.categories?.length ?? 0) + (purchaseSummary?.categories?.length ?? 0)}
                </dd>
              </div>
            </dl>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
