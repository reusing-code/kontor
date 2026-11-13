import { useTranslation } from "react-i18next"
import { AlertTriangle } from "lucide-react"
import { useCapabilities } from "@/hooks/use-capabilities"
import { useBankConnections } from "@/modules/ledger/hooks/use-bank-connections"
import { useStartBankAuthorization } from "@/modules/ledger/hooks/use-bank-authorization"
import { formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import { Button } from "@/components/ui/button"

// Warns while a bank consent is about to end or has ended; shows nothing for
// active consents or without the bank import capability.
export function BankConsentBanner() {
  const { t } = useTranslation()
  const { bankImport } = useCapabilities()
  const { data: connections = [] } = useBankConnections()
  const { start, isPending } = useStartBankAuthorization()

  const affected = connections.filter((c) => c.status !== "active")
  if (!bankImport || affected.length === 0) return null

  return (
    <div role="alert" className="space-y-2 rounded-md border border-amber-500/50 bg-amber-500/10 p-4">
      {affected.map((connection) => (
        <div key={connection.id} className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 text-sm">
            <AlertTriangle className="h-4 w-4 shrink-0 text-amber-600" />
            <span>
              {connection.status === "expired"
                ? t("ledger.bankImport.banner.expired", { bank: connection.aspspName })
                : t("ledger.bankImport.banner.expiring", { bank: connection.aspspName, date: formatLedgerDate(connection.expiresAt) })}
            </span>
          </div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={isPending}
            onClick={() => start({ aspspName: connection.aspspName, aspspCountry: connection.aspspCountry })}
          >
            {t("ledger.bankImport.reauthorize")}
          </Button>
        </div>
      ))}
    </div>
  )
}
