import { useTranslation } from "react-i18next"
import type { VehicleSummary } from "@/modules/auto/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface VehicleDashboardProps {
  summary: VehicleSummary
}

export function VehicleDashboard({ summary }: VehicleDashboardProps) {
  const { t } = useTranslation()
  const currency = t("common.currency")
  const fmt = (n: number) => n.toFixed(2)
  const fmtInt = (n: number) => Math.round(n).toLocaleString()

  const costTypeKeys = ["service", "fuel", "insurance", "tax", "inspection", "tires", "misc"] as const

  return (
    <div className="space-y-6">
      {/* Overview cards */}
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

      {/* Secondary stats */}
      <div className="grid gap-4 sm:grid-cols-3">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t("vehicleSummary.monthsOwned")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-semibold">{summary.monthsOwned.toFixed(1)}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t("vehicleSummary.kmPerMonth")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-semibold">{summary.kmPerMonth > 0 ? `${fmtInt(summary.kmPerMonth)} km` : "-"}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              {t("vehicleSummary.entryCount")}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-lg font-semibold">{summary.entryCount}</div>
          </CardContent>
        </Card>
      </div>

      {/* Costs by type */}
      {Object.keys(summary.costsByType).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("vehicleSummary.costsByType")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              {costTypeKeys.map((type) => {
                const val = summary.costsByType[type]
                if (!val) return null
                return (
                  <div key={type} className="flex justify-between rounded-md border p-3">
                    <span className="text-sm text-muted-foreground">{t(`costTypes.${type}`)}</span>
                    <span className="text-sm font-medium">{fmt(val)} {currency}</span>
                  </div>
                )
              })}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Costs by year */}
      {summary.costsByYear.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("vehicleSummary.costsByYear")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("vehicleSummary.year")}</TableHead>
                    {costTypeKeys.map((type) => (
                      <TableHead key={type} className="text-right">{t(`costTypes.${type}`)}</TableHead>
                    ))}
                    <TableHead className="text-right font-bold">{t("vehicleSummary.total")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {summary.costsByYear.map((yc) => (
                    <TableRow key={yc.year}>
                      <TableCell className="font-medium">{yc.year}</TableCell>
                      <TableCell className="text-right">{yc.service > 0 ? fmt(yc.service) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.fuel > 0 ? fmt(yc.fuel) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.insurance > 0 ? fmt(yc.insurance) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.tax > 0 ? fmt(yc.tax) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.inspection > 0 ? fmt(yc.inspection) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.tires > 0 ? fmt(yc.tires) : "-"}</TableCell>
                      <TableCell className="text-right">{yc.misc > 0 ? fmt(yc.misc) : "-"}</TableCell>
                      <TableCell className="text-right font-bold">{fmt(yc.total)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Mileage by year */}
      {summary.mileageByYear && summary.mileageByYear.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>{t("vehicleSummary.mileageByYear")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("vehicleSummary.year")}</TableHead>
                    <TableHead className="text-right">{t("vehicleSummary.km")}</TableHead>
                    <TableHead className="text-right">{t("vehicleSummary.fuelCostPerKm")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {summary.mileageByYear.map((ym) => (
                    <TableRow key={ym.year}>
                      <TableCell className="font-medium">{ym.year}</TableCell>
                      <TableCell className="text-right">{fmtInt(ym.km)} km</TableCell>
                      <TableCell className="text-right">{ym.fuelCostPerKm > 0 ? `${fmt(ym.fuelCostPerKm)} ${currency}` : "-"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Projection */}
      {summary.projection && (
        <Card>
          <CardHeader>
            <CardTitle>{t("vehicleSummary.projection")}</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {summary.projection.targetMonths != null && (
                <div>
                  <div className="text-xs text-muted-foreground">{t("vehicleSummary.targetMonths")}</div>
                  <div className="text-sm font-medium">{summary.projection.targetMonths}</div>
                </div>
              )}
              {summary.projection.targetMileage != null && (
                <div>
                  <div className="text-xs text-muted-foreground">{t("vehicleSummary.targetMileage")}</div>
                  <div className="text-sm font-medium">{fmtInt(summary.projection.targetMileage)} km</div>
                </div>
              )}
              <div>
                <div className="text-xs text-muted-foreground">{t("vehicleSummary.projectedTotalCost")}</div>
                <div className="text-sm font-medium">{fmt(summary.projection.projectedTotalCost)} {currency}</div>
              </div>
              <div>
                <div className="text-xs text-muted-foreground">{t("vehicleSummary.projectedCostPerMonth")}</div>
                <div className="text-sm font-medium">{fmt(summary.projection.projectedCostPerMonth)} {currency}</div>
              </div>
              {summary.projection.projectedCostPerKm > 0 && (
                <div>
                  <div className="text-xs text-muted-foreground">{t("vehicleSummary.projectedCostPerKm")}</div>
                  <div className="text-sm font-medium">{fmt(summary.projection.projectedCostPerKm)} {currency}</div>
                </div>
              )}
              {summary.projection.theoreticalResidualValue > 0 && (
                <div>
                  <div className="text-xs text-muted-foreground">{t("vehicleSummary.theoreticalResidual")}</div>
                  <div className="text-sm font-medium">{fmt(summary.projection.theoreticalResidualValue)} {currency}</div>
                </div>
              )}
              {summary.projection.requiredSalePrice > 0 && (
                <div>
                  <div className="text-xs text-muted-foreground">{t("vehicleSummary.requiredSalePrice")}</div>
                  <div className="text-sm font-medium">{fmt(summary.projection.requiredSalePrice)} {currency}</div>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
