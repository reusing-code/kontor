import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useVehicles } from "@/modules/auto/hooks/use-vehicles"
import { cn } from "@/lib/utils"
import { SidebarSection } from "@/components/sidebar"

export function AutoSidebarSection() {
  const { t } = useTranslation()
  const { data: vehicles = [] } = useVehicles()
  const matchRoute = useMatchRoute()

  return (
    <SidebarSection
      title={t("nav.auto")}
      to="/auto"
      isActive={!!matchRoute({ to: "/auto", fuzzy: true })}
    >
      {vehicles.map((vehicle) => {
        const active = matchRoute({
          to: "/auto/vehicles/$vehicleId",
          params: { vehicleId: vehicle.id },
        })
        return (
          <Link
            key={vehicle.id}
            to="/auto/vehicles/$vehicleId"
            params={{ vehicleId: vehicle.id }}
            className={cn(
              "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
              active && "bg-accent font-medium",
            )}
          >
            {vehicle.name}
          </Link>
        )
      })}
    </SidebarSection>
  )
}
