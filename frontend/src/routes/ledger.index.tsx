import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerAccounts, useLedgerImports } from "@/hooks/use-ledger"
import { LedgerAccountList } from "@/components/ledger-account-list"
import { LedgerImportPanel } from "@/components/ledger-import-panel"
import { LedgerImportsList } from "@/components/ledger-imports-list"
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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("nav.ledger")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.description")}</p>
      </div>

      <LedgerImportPanel accounts={accounts} />
      <LedgerAccountList accounts={accounts} />
      <LedgerImportsList imports={imports} accounts={accounts} />
    </div>
  )
}
