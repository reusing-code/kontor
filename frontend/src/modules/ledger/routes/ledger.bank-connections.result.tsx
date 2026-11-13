import { Link, useSearch } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { CheckCircle2, XCircle } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { parseBankResult } from "@/modules/ledger/lib/bank-result"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function LedgerBankResultPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.bankImport.result.title"), t("app.title"))
  const result = parseBankResult(useSearch({ strict: false }))
  const connected = result.bank === "connected"

  return (
    <Card className="max-w-xl">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {connected ? <CheckCircle2 className="h-5 w-5 text-green-600" /> : <XCircle className="h-5 w-5 text-destructive" />}
          {connected ? t("ledger.bankImport.result.connectedTitle") : t("ledger.bankImport.result.failedTitle")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm">
          {connected
            ? t("ledger.bankImport.result.connected")
            : t(`ledger.bankImport.result.reasons.${result.reason ?? "generic"}`)}
        </p>
        <Button asChild>
          <Link to="/ledger/bank-connections">{t("ledger.bankImport.result.back")}</Link>
        </Button>
      </CardContent>
    </Card>
  )
}
