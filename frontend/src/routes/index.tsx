import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useQuery } from "@tanstack/react-query"
import { rootRoute } from "./__root"
import { modules } from "@/modules/registry"
import { useModules } from "@/hooks/use-modules"
import { getSummary } from "@/modules/contracts/lib/contract-repository"
import { getPurchaseSummary } from "@/modules/purchases/lib/purchase-repository"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { CalendarClock } from "lucide-react"

export const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const { t } = useTranslation()
  usePageTitle(t("home.title"), t("app.title"))
  const { isEnabled } = useModules()

  return (
    <div className="space-y-8">
      <h1 className="text-2xl font-bold">{t("home.title")}</h1>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        {modules
          .filter((m) => isEnabled(m.id))
          .map((m) => (m.HomeCard ? <m.HomeCard key={m.id} /> : null))}
      </div>

      <div className="grid gap-6 sm:grid-cols-3">
        <QuickStatsCard />
      </div>
    </div>
  )
}

function QuickStatsCard() {
  const { t } = useTranslation()
  const { isEnabled } = useModules()
  const contractsEnabled = isEnabled("contracts")
  const purchasesEnabled = isEnabled("purchases")

  const { data: contractSummary } = useQuery({
    queryKey: ["summary"],
    queryFn: getSummary,
    enabled: contractsEnabled,
  })

  const { data: purchaseSummary } = useQuery({
    queryKey: ["purchases-summary"],
    queryFn: getPurchaseSummary,
    enabled: purchasesEnabled,
  })

  if (!contractsEnabled && !purchasesEnabled) {
    return null
  }

  const categoryCount =
    (contractsEnabled ? contractSummary?.categories?.length ?? 0 : 0) +
    (purchasesEnabled ? purchaseSummary?.categories?.length ?? 0 : 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <CalendarClock className="h-4 w-4" />
          {t("home.quickStats")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="space-y-2 text-sm">
          {contractsEnabled && (
            <div className="flex justify-between">
              <dt className="text-muted-foreground">{t("home.yearlyContracts")}</dt>
              <dd className="font-medium">
                {(contractSummary?.totalYearlyAmount ?? 0).toFixed(2)} {t("common.currency")}
              </dd>
            </div>
          )}
          <div className="flex justify-between">
            <dt className="text-muted-foreground">{t("home.categories")}</dt>
            <dd className="font-medium">{categoryCount}</dd>
          </div>
        </dl>
      </CardContent>
    </Card>
  )
}
