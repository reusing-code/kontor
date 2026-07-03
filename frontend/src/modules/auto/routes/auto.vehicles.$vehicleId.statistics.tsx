import { getRouteApi, Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { ArrowLeft } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useVehicle, useVehicleSummary } from "@/modules/auto/hooks/use-vehicles"
import { Button } from "@/components/ui/button"
import { VehicleDashboard } from "@/modules/auto/components/vehicle-dashboard"
import { VehicleStatistics } from "@/modules/auto/components/vehicle-statistics"

const routeApi = getRouteApi("/auto/vehicles/$vehicleId/statistics")

export function AutoVehicleStatisticsPage() {
  const { t } = useTranslation()
  const { vehicleId } = routeApi.useParams()
  const { data: vehicle } = useVehicle(vehicleId)
  const { data: summary } = useVehicleSummary(vehicleId)

  usePageTitle(
    vehicle ? `${vehicle.name} – ${t("vehicleSummary.statistics")}` : t("vehicleSummary.statistics"),
    t("app.title"),
  )

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" asChild>
          <Link to="/auto/vehicles/$vehicleId" params={{ vehicleId }}>
            <ArrowLeft className="h-4 w-4" />
          </Link>
        </Button>
        <h1 className="text-2xl font-bold">
          {vehicle?.name ?? "..."} – {t("vehicleSummary.statistics")}
        </h1>
      </div>

      {summary && (
        <>
          <VehicleDashboard summary={summary} />
          <VehicleStatistics summary={summary} />
        </>
      )}
    </div>
  )
}
