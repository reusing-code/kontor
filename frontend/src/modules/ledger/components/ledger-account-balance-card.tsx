import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { AlertTriangle } from "lucide-react"
import type { LedgerAccount } from "@/modules/ledger/types"
import { useDeleteLedgerAccountBalance, useSetLedgerAccountBalance } from "@/modules/ledger/hooks/use-ledger"
import { formatAmountMinor, formatSourceType } from "@/modules/ledger/lib/ledger-utils"
import { hasDrift } from "@/modules/ledger/lib/balance"
import { LedgerBalanceDialog } from "@/modules/ledger/components/ledger-balance-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function LedgerAccountBalanceCard({ account }: { account: LedgerAccount }) {
  const { t, i18n } = useTranslation()
  const [setting, setSetting] = useState(false)
  const setBalance = useSetLedgerAccountBalance()
  const deleteBalance = useDeleteLedgerAccountBalance()
  const balance = account.balance
  const money = (minor: number) => formatAmountMinor(minor, account.currency, i18n.language)
  const source = (value: string) => (value === "manual" ? t("ledger.balance.sourceManual") : formatSourceType(value))

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-2">
        <CardTitle>{t("ledger.balance.title")}</CardTitle>
        <div className="flex flex-wrap gap-2">
          {balance?.source === "manual" && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              aria-label={t("ledger.balance.removeManualLabel")}
              disabled={deleteBalance.isPending}
              onClick={() =>
                deleteBalance.mutate(
                  { id: account.id, asOf: balance.asOf },
                  { onSuccess: () => toast.success(t("ledger.balance.removed")) },
                )
              }
            >
              {t("ledger.balance.removeManual")}
            </Button>
          )}
          <Button type="button" variant="outline" size="sm" onClick={() => setSetting(true)}>
            {t("ledger.balance.set")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2">
        {balance ? (
          <>
            <div>
              <div className="text-xs text-muted-foreground">{t("ledger.balance.current")}</div>
              <div className={balance.currentMinor < 0 ? "text-2xl font-semibold text-destructive" : "text-2xl font-semibold"}>{money(balance.currentMinor)}</div>
            </div>
            <div className="text-xs text-muted-foreground">
              {t("ledger.balance.reported", { date: balance.asOf, source: source(balance.source) })}
            </div>
            {hasDrift(balance.driftMinor) && (
              <div className="flex flex-wrap items-start gap-2 rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm" role="status">
                <Badge variant="outline" className="gap-1">
                  <AlertTriangle className="h-3 w-3" aria-hidden />
                  {t("ledger.balance.drift", { amount: money(balance.driftMinor) })}
                </Badge>
                <span className="text-muted-foreground">{t("ledger.balance.driftHint", { date: balance.asOf, previous: balance.previousAsOf })}</span>
              </div>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">{t("ledger.balance.none")}</p>
        )}
      </CardContent>
      <LedgerBalanceDialog
        open={setting}
        onOpenChange={setSetting}
        pending={setBalance.isPending}
        onSubmit={(data) =>
          setBalance.mutate(
            { id: account.id, data },
            {
              onSuccess: () => {
                setSetting(false)
                toast.success(t("ledger.balance.saved"))
              },
            },
          )
        }
      />
    </Card>
  )
}
