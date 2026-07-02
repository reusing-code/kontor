import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { getPurchaseSummary } from "@/modules/purchases/lib/purchase-repository"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { ShoppingBag, ArrowRight } from "lucide-react"

export function PurchasesHomeCard() {
  const { t } = useTranslation()

  const { data: purchaseSummary } = useQuery({
    queryKey: ["purchases-summary"],
    queryFn: getPurchaseSummary,
  })

  return (
    <Link to="/purchases" className="group">
      <Card className="transition-colors group-hover:bg-accent/50">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <ShoppingBag className="h-5 w-5 text-primary" />
              </div>
              <CardTitle className="text-xl">{t("nav.purchases")}</CardTitle>
            </div>
            <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-2xl font-bold">{purchaseSummary?.totalPurchases ?? 0}</p>
              <p className="text-sm text-muted-foreground">{t("home.totalPurchases")}</p>
            </div>
            <div>
              <p className="text-2xl font-bold">
                {(purchaseSummary?.totalSpent ?? 0).toFixed(2)} {t("common.currency")}
              </p>
              <p className="text-sm text-muted-foreground">{t("home.totalSpent")}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
