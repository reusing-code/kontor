import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { useUpcomingRenewals } from "@/modules/contracts/hooks/use-contracts"
import { useSettings } from "@/hooks/use-settings"
import { updateContract, deleteContract } from "@/modules/contracts/lib/contract-repository"
import { useMutation, useQueryClient } from "@tanstack/react-query"
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
  const qc = useQueryClient()

  const [editingContract, setEditingContract] = useState<Contract | null>(null)
  const [deletingContract, setDeletingContract] = useState<Contract | null>(null)

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: ContractFormData }) => updateContract(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      toast.success(t("contract.updated"))
      setEditingContract(null)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: ({ id }: { id: string }) => deleteContract(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      toast.success(t("contract.deleted"))
      setDeletingContract(null)
    },
  })

  function handleUpdate(data: ContractFormData) {
    if (!editingContract) return
    updateMutation.mutate({ id: editingContract.id, data })
  }

  function handleDelete() {
    if (!deletingContract) return
    deleteMutation.mutate({ id: deletingContract.id })
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
