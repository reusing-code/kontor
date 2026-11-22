import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { AlertTriangle } from "lucide-react"
import type { LedgerAccount } from "@/modules/ledger/types"
import { formatAmountMinor } from "@/modules/ledger/lib/ledger-utils"
import { hasDrift } from "@/modules/ledger/lib/balance"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface LedgerAccountListProps {
  accounts: LedgerAccount[]
}

export function LedgerAccountList({ accounts }: LedgerAccountListProps) {
  const { t, i18n } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.accounts")}</CardTitle>
      </CardHeader>
      <CardContent>
        {accounts.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("ledger.noAccounts")}</p>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
            {accounts.map((account) => (
              <Link key={account.id} to="/ledger/accounts/$accountId" params={{ accountId: account.id }}>
                <div className="rounded-lg border p-4 transition-colors hover:bg-accent/40">
                  <div className="font-medium">{account.name}</div>
                  <div className="mt-1 text-sm text-muted-foreground">{account.bank}</div>
                  <div className="mt-2 text-xs text-muted-foreground">{account.iban || t("ledger.noIban")}</div>
                  <div className="mt-2 flex items-center gap-2 text-sm">
                    <span className="sr-only">{t("ledger.balance.current")}: </span>
                    <span className={account.balance && account.balance.currentMinor < 0 ? "font-semibold text-destructive" : "font-semibold"}>
                      {account.balance ? formatAmountMinor(account.balance.currentMinor, account.currency, i18n.language) : "–"}
                    </span>
                    {account.balance && hasDrift(account.balance.driftMinor) && (
                      <AlertTriangle className="h-4 w-4 text-yellow-600" aria-label={t("ledger.balance.drift", { amount: formatAmountMinor(account.balance.driftMinor, account.currency, i18n.language) })} />
                    )}
                  </div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
