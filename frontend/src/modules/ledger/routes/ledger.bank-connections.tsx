import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { BankConnectionsPanel } from "@/modules/ledger/components/bank-connections-panel"

export function LedgerBankConnectionsPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.bankImport.title"), t("app.title"))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.bankImport.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.bankImport.description")}</p>
      </div>
      <BankConnectionsPanel />
    </div>
  )
}
