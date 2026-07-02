import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { useCategoryContracts, useCreateContract, useUpdateContract, useDeleteContract } from "@/modules/contracts/hooks/use-contracts"
import { getCategoryById } from "@/lib/category-repository"
import { useQuery } from "@tanstack/react-query"
import type { Contract, ContractFormData } from "@/modules/contracts/types"
import { Button } from "@/components/ui/button"
import { ContractsTable } from "@/modules/contracts/components/contracts-table"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

export const contractsCategoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts/categories/$categoryId",
  beforeLoad: moduleGuard("contracts"),
  component: ContractsCategoryDetailPage,
})

function ContractsCategoryDetailPage() {
  const { t } = useTranslation()
  const { categoryId } = contractsCategoryRoute.useParams()
  const { data: category } = useQuery({
    queryKey: ["category", "contracts", categoryId],
    queryFn: () => getCategoryById("contracts", categoryId),
  })
  const categoryName = category ? (category.nameKey ? t(category.nameKey) : category.name) : t("nav.categories")
  usePageTitle(categoryName, t("app.title"))
  const { data: contracts = [] } = useCategoryContracts(categoryId)
  const createContract = useCreateContract(categoryId)
  const updateContract = useUpdateContract(categoryId)
  const deleteContract = useDeleteContract(categoryId)

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingContract, setEditingContract] = useState<Contract | null>(null)
  const [deletingContract, setDeletingContract] = useState<Contract | null>(null)

  function handleCreate(data: ContractFormData) {
    createContract.mutate(data, { onSuccess: () => toast.success(t("contract.created")) })
  }

  function handleUpdate(data: ContractFormData) {
    if (!editingContract) return
    updateContract.mutate(
      { id: editingContract.id, data },
      { onSuccess: () => toast.success(t("contract.updated")) },
    )
    setEditingContract(null)
  }

  function handleDelete() {
    if (!deletingContract) return
    deleteContract.mutate(deletingContract.id, {
      onSuccess: () => toast.success(t("contract.deleted")),
    })
    setDeletingContract(null)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h1 className="text-2xl font-bold">
          {category ? (category.nameKey ? t(category.nameKey) : category.name) : "..."}
        </h1>
        <div className="ml-auto">
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("contract.create")}
          </Button>
        </div>
      </div>

      <ContractsTable
        contracts={contracts}
        onEdit={(c) => setEditingContract(c)}
        onDelete={(c) => setDeletingContract(c)}
      />

      <ContractDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={handleCreate}
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
