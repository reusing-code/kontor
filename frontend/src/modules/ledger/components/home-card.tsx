import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useLedgerAccounts, useLedgerImports, useLedgerReviewQueue } from "@/modules/ledger/hooks/use-ledger"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { Landmark, ArrowRight } from "lucide-react"

export function LedgerHomeCard() {
  const { t } = useTranslation()

  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const { data: ledgerImports = [] } = useLedgerImports()
  const { data: ledgerReviewPage } = useLedgerReviewQueue(10)

  return (
    <Link to="/ledger" className="group">
      <Card className="transition-colors group-hover:bg-accent/50">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <Landmark className="h-5 w-5 text-primary" />
              </div>
              <CardTitle className="text-xl">{t("nav.ledger")}</CardTitle>
            </div>
            <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-2xl font-bold">{ledgerAccounts.length}</p>
              <p className="text-sm text-muted-foreground">{t("ledger.accounts")}</p>
            </div>
            <div>
              <p className="text-2xl font-bold">{ledgerImports.length}</p>
              <p className="text-sm text-muted-foreground">{t("ledger.importHistory")}</p>
            </div>
            <div>
              <p className="text-2xl font-bold">{ledgerReviewPage?.items.length ?? 0}</p>
              <p className="text-sm text-muted-foreground">{t("ledger.reviewQueue")}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
