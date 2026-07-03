import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { LedgerEmailAccountsPanel } from "@/modules/ledger/components/ledger-email-accounts-panel"

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
