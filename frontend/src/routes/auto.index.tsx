import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus, MoreVertical } from "lucide-react"
import { useNavigate } from "@tanstack/react-router"
import { rootRoute } from "./__root"
import { useVehicles, useCreateVehicle, useUpdateVehicle, useDeleteVehicle } from "@/hooks/use-vehicles"
import type { Vehicle, VehicleFormData } from "@/types/vehicle"
import { Button } from "@/components/ui/button"
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { VehicleDialog } from "@/components/vehicle-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

export const autoIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto",
  component: AutoIndexPage,
})

export function AutoIndexPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.auto"), t("app.title"))
  const navigate = useNavigate()
  const { data: vehicles = [] } = useVehicles()
  const createVehicle = useCreateVehicle()
  const updateVehicle = useUpdateVehicle()
  const deleteVehicle = useDeleteVehicle()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingVehicle, setEditingVehicle] = useState<Vehicle | null>(null)
  const [deletingVehicle, setDeletingVehicle] = useState<Vehicle | null>(null)

  function handleCreate(data: VehicleFormData) {
    createVehicle.mutate(data, { onSuccess: () => toast.success(t("vehicle.created")) })
  }

  function handleUpdate(data: VehicleFormData) {
    if (!editingVehicle) return
    updateVehicle.mutate(
      { id: editingVehicle.id, data },
      { onSuccess: () => toast.success(t("vehicle.updated")) },
    )
    setEditingVehicle(null)
  }

  function handleDelete() {
    if (!deletingVehicle) return
    deleteVehicle.mutate(deletingVehicle.id, {
      onSuccess: () => toast.success(t("vehicle.deleted")),
    })
    setDeletingVehicle(null)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("nav.auto")}</h1>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          {t("vehicle.create")}
        </Button>
      </div>

      {vehicles.length === 0 ? (
        <p className="text-muted-foreground">{t("vehicle.noVehicles")}</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {vehicles.map((vehicle) => (
            <Card
              key={vehicle.id}
              className="cursor-pointer transition-colors hover:bg-accent/50"
              onClick={() => navigate({ to: "/auto/vehicles/$vehicleId", params: { vehicleId: vehicle.id } })}
            >
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-lg font-semibold">{vehicle.name}</CardTitle>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild onClick={(e) => e.stopPropagation()}>
                    <Button variant="ghost" size="icon" className="h-8 w-8">
                      <MoreVertical className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" onClick={(e) => e.stopPropagation()}>
                    <DropdownMenuItem onClick={() => setEditingVehicle(vehicle)}>
                      {t("common.edit")}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setDeletingVehicle(vehicle)} className="text-destructive">
                      {t("common.delete")}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </CardHeader>
              <CardContent>
                <p className="text-sm text-muted-foreground">
                  {[vehicle.make, vehicle.model, vehicle.year].filter(Boolean).join(" ") || vehicle.licensePlate || ""}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <VehicleDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={handleCreate}
      />

      <VehicleDialog
        open={!!editingVehicle}
        onOpenChange={(open) => { if (!open) setEditingVehicle(null) }}
        vehicle={editingVehicle}
        onSubmit={handleUpdate}
      />

      <DeleteConfirmDialog
        open={!!deletingVehicle}
        onOpenChange={(open) => { if (!open) setDeletingVehicle(null) }}
        description={t("vehicle.deleteConfirm", { name: deletingVehicle?.name ?? "" })}
        onConfirm={handleDelete}
      />
    </div>
  )
}
