import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { rootRoute } from "./__root"
import {
  useVehicle,
  useVehicleSummary,
  useCostEntries,
  useCreateCostEntry,
  useUpdateCostEntry,
  useDeleteCostEntry,
  useUpdateVehicle,
} from "@/hooks/use-vehicles"
import type { CostEntry, CostEntryFormData, VehicleFormData } from "@/types/vehicle"
import { Button } from "@/components/ui/button"
import { CostEntriesTable } from "@/components/cost-entries-table"
import { CostEntryDialog } from "@/components/cost-entry-dialog"
import { VehicleDashboard } from "@/components/vehicle-dashboard"
import { VehicleDialog } from "@/components/vehicle-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { LinkedTransactionsList } from "@/components/linked-transactions-list"
import { Separator } from "@/components/ui/separator"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export const autoVehicleDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/auto/vehicles/$vehicleId",
  component: AutoVehicleDetailPage,
})

function AutoVehicleDetailPage() {
  const { t } = useTranslation()
  const { vehicleId } = autoVehicleDetailRoute.useParams()
  const { data: vehicle } = useVehicle(vehicleId)
  const { data: summary } = useVehicleSummary(vehicleId)
  const { data: costEntries = [] } = useCostEntries(vehicleId)
  const createCostEntry = useCreateCostEntry(vehicleId)
  const updateCostEntry = useUpdateCostEntry(vehicleId)
  const deleteCostEntry = useDeleteCostEntry(vehicleId)
  const updateVehicle = useUpdateVehicle()

  usePageTitle(vehicle?.name ?? t("nav.auto"), t("app.title"))

  const [costDialogOpen, setCostDialogOpen] = useState(false)
  const [editingEntry, setEditingEntry] = useState<CostEntry | null>(null)
  const [deletingEntry, setDeletingEntry] = useState<CostEntry | null>(null)
  const [editingVehicle, setEditingVehicle] = useState(false)

  function handleCreateCost(data: CostEntryFormData) {
    createCostEntry.mutate(data, { onSuccess: () => toast.success(t("costEntry.created")) })
  }

  function handleUpdateCost(data: CostEntryFormData) {
    if (!editingEntry) return
    updateCostEntry.mutate(
      { id: editingEntry.id, data },
      { onSuccess: () => toast.success(t("costEntry.updated")) },
    )
    setEditingEntry(null)
  }

  function handleDeleteCost() {
    if (!deletingEntry) return
    deleteCostEntry.mutate(deletingEntry.id, {
      onSuccess: () => toast.success(t("costEntry.deleted")),
    })
    setDeletingEntry(null)
  }

  function handleUpdateVehicle(data: VehicleFormData) {
    if (!vehicle) return
    updateVehicle.mutate(
      { id: vehicle.id, data },
      { onSuccess: () => toast.success(t("vehicle.updated")) },
    )
    setEditingVehicle(false)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h1 className="text-2xl font-bold">{vehicle?.name ?? "..."}</h1>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" onClick={() => setEditingVehicle(true)}>
            {t("vehicle.editVehicle")}
          </Button>
          <Button onClick={() => setCostDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("costEntry.create")}
          </Button>
        </div>
      </div>

      {summary && <VehicleDashboard summary={summary} />}

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.linkedTransactions")}</CardTitle>
        </CardHeader>
        <CardContent>
          <LinkedTransactionsList transactionIds={vehicle?.linkedTransactionIds ?? []} />
        </CardContent>
      </Card>

      <Separator />

      <h2 className="text-xl font-semibold">{t("costEntry.entries")}</h2>

      <CostEntriesTable
        entries={costEntries}
        onEdit={(e) => setEditingEntry(e)}
        onDelete={(e) => setDeletingEntry(e)}
      />

      <CostEntryDialog
        open={costDialogOpen}
        onOpenChange={setCostDialogOpen}
        onSubmit={handleCreateCost}
      />

      <CostEntryDialog
        open={!!editingEntry}
        onOpenChange={(open) => { if (!open) setEditingEntry(null) }}
        costEntry={editingEntry}
        onSubmit={handleUpdateCost}
      />

      <VehicleDialog
        open={editingVehicle}
        onOpenChange={setEditingVehicle}
        vehicle={vehicle}
        onSubmit={handleUpdateVehicle}
      />

      <DeleteConfirmDialog
        open={!!deletingEntry}
        onOpenChange={(open) => { if (!open) setDeletingEntry(null) }}
        description={t("costEntry.deleteConfirm", { name: deletingEntry?.description ?? deletingEntry?.type ?? "" })}
        onConfirm={handleDeleteCost}
      />
    </div>
  )
}
