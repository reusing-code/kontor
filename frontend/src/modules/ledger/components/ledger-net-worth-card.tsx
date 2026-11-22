import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import type { LedgerAccount } from "@/modules/ledger/types"
import { formatAmountMinor } from "@/modules/ledger/lib/ledger-utils"
import { netWorthFromAccounts } from "@/modules/ledger/lib/net-worth"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

// LedgerNetWorthCard shows the sum of the current balances on the ledger
// dashboard and links to the analysis page. It works on the account list the
// dashboard already has and must not import anything chart related. While
// that list is loading it shows no statement about the accounts.
export function LedgerNetWorthCard({ accounts, isPending = false }: { accounts: LedgerAccount[]; isPending?: boolean }) {
  const { t, i18n } = useTranslation()
  const netWorth = netWorthFromAccounts(accounts)
  const hasBalance = netWorth.included > 0

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <CardTitle>{t("ledger.analysis.card.title")}</CardTitle>
        <Button asChild variant="outline">
          <Link to="/ledger/analysis">{t("ledger.analysis.card.open")}</Link>
        </Button>
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-semibold">{hasBalance ? formatAmountMinor(netWorth.totalMinor, "EUR", i18n.language) : "–"}</div>
        {!isPending && (
          <p className="text-sm text-muted-foreground">
            {hasBalance ? t("ledger.analysis.card.accounts", { count: netWorth.included }) : t("ledger.analysis.card.none")}
          </p>
        )}
        {netWorth.excluded.length > 0 && (
          <p className="text-sm text-muted-foreground">{t("ledger.analysis.card.excluded", { count: netWorth.excluded.length })}</p>
        )}
      </CardContent>
    </Card>
  )
}
