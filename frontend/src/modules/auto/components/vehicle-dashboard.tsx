import { useTranslation } from "react-i18next"
import type { VehicleSummary } from "@/modules/auto/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface VehicleDashboardProps {
  summary: VehicleSummary
}

export function VehicleDashboard({ summary }: VehicleDashboardProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const fmt = (n: number) => n.toFixed(2)
  const fmtInt = (n: number) => Math.round(n).toLocaleString()

  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("vehicleSummary.totalCost")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{fmt(summary.totalCost)} {currency}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("vehicleSummary.costPerMonth")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{fmt(summary.costPerMonth)} {currency}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("vehicleSummary.costPerKm")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{summary.costPerKm > 0 ? `${fmt(summary.costPerKm)} ${currency}` : "-"}</div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            {t("vehicleSummary.currentMileage")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{summary.currentMileage > 0 ? `${fmtInt(summary.currentMileage)} km` : "-"}</div>
        </CardContent>
      </Card>
    </div>
  )
}
