import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import type { LedgerAccount } from "@/modules/ledger/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface LedgerAccountListProps {
  accounts: LedgerAccount[]
}

export function LedgerAccountList({ accounts }: LedgerAccountListProps) {
  const { t } = useTranslation()

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
                </div>
              </Link>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
