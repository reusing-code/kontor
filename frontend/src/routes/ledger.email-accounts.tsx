import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { LedgerEmailAccountsPanel } from "@/components/ledger-email-accounts-panel"
import { rootRoute } from "./__root"

export const ledgerEmailAccountsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/email-accounts",
  component: LedgerEmailAccountsPage,
})

export function LedgerEmailAccountsPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.email.accounts"), t("app.title"))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.email.accounts")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.email.description")}</p>
      </div>
      <LedgerEmailAccountsPanel />
    </div>
  )
}
