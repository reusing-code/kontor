import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { useDeleteContract, useUpcomingRenewals, useUpdateContract } from "@/modules/contracts/hooks/use-contracts"
import { useSettings } from "@/hooks/use-settings"
import type { Contract, ContractFormData } from "@/modules/contracts/types"
import { ContractsTable } from "@/modules/contracts/components/contracts-table"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { getRenewalRowClass } from "@/lib/utils"

export const contractsUpcomingRenewalsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts/upcoming-renewals",
  beforeLoad: moduleGuard("contracts"),
  component: UpcomingRenewalsPage,
})

function UpcomingRenewalsPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.upcomingRenewals"), t("app.title"))
  const { data: settings } = useSettings()
  const { data: contracts = [] } = useUpcomingRenewals(settings?.renewalDays)

  const [editingContract, setEditingContract] = useState<Contract | null>(null)
  const [deletingContract, setDeletingContract] = useState<Contract | null>(null)

  const updateMutation = useUpdateContract()

  const deleteMutation = useDeleteContract()

  function handleUpdate(data: ContractFormData) {
    if (!editingContract) return Promise.resolve()
    return updateMutation.mutateAsync({ id: editingContract.id, data }, { onSuccess: () => toast.success(t("contract.updated")) })
  }

  function handleDelete() {
    if (!deletingContract) return
    deleteMutation.mutate(deletingContract.id, {
      onSuccess: () => {
        toast.success(t("contract.deleted"))
        setDeletingContract(null)
      },
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h1 className="text-2xl font-bold">{t("nav.upcomingRenewals")}</h1>
      </div>

      <ContractsTable
        contracts={contracts}
        onEdit={(c) => setEditingContract(c)}
        onDelete={(c) => setDeletingContract(c)}
        getRowClassName={getRenewalRowClass}
      />

      <ContractDialog
        open={!!editingContract}
        onOpenChange={(open) => { if (!open) setEditingContract(null) }}
        contract={editingContract}
        onSubmit={handleUpdate}
      />

      <DeleteConfirmDialog
        open={!!deletingContract}
        onOpenChange={(open) => { if (!open) setDeletingContract(null) }}
        description={t("contract.deleteConfirm", { name: deletingContract?.name ?? "" })}
        onConfirm={handleDelete}
      />
    </div>
  )
}
