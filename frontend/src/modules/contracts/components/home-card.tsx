import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { getSummary } from "@/modules/contracts/lib/contract-repository"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import { FileText, ArrowRight } from "lucide-react"

export function ContractsHomeCard() {
  const { t } = useTranslation()

  const { data: contractSummary } = useQuery({
    queryKey: ["summary"],
    queryFn: getSummary,
  })

  return (
    <Link to="/contracts" className="group">
      <Card className="transition-colors group-hover:bg-accent/50">
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10">
                <FileText className="h-5 w-5 text-primary" />
              </div>
              <CardTitle className="text-xl">{t("nav.contracts")}</CardTitle>
            </div>
            <ArrowRight className="h-5 w-5 text-muted-foreground transition-transform group-hover:translate-x-1" />
          </div>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-2xl font-bold">{contractSummary?.totalContracts ?? 0}</p>
              <p className="text-sm text-muted-foreground">{t("home.activeContracts")}</p>
            </div>
            <div>
              <p className="text-2xl font-bold">
                {(contractSummary?.totalMonthlyAmount ?? 0).toFixed(2)} {t("common.currency")}
              </p>
              <p className="text-sm text-muted-foreground">{t("home.monthlySpend")}</p>
            </div>
          </div>
        </CardContent>
      </Card>
    </Link>
  )
}
