import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerEmailOrders } from "@/modules/ledger/hooks/use-ledger"
import { LedgerEmailLlmNotice } from "@/modules/ledger/components/ledger-email-llm-notice"
import { LedgerEmailOrdersTable } from "@/modules/ledger/components/ledger-email-orders-table"

export function LedgerEmailOrdersPage() {
  const { t } = useTranslation()
  const { data: orders = [] } = useLedgerEmailOrders()
  usePageTitle(t("ledger.email.orders"), t("app.title"))

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.email.orders")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.email.ordersDescription")}</p>
      </div>
      <LedgerEmailLlmNotice />
      <LedgerEmailOrdersTable orders={orders} />
    </div>
  )
}
