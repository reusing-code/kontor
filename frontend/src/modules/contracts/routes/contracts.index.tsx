import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus, Upload } from "lucide-react"
import { useCategories, useCreateCategory, useUpdateCategory, useDeleteCategory } from "@/hooks/use-categories"
import { useUpcomingRenewals } from "@/modules/contracts/hooks/use-contracts"
import { useSettings } from "@/hooks/use-settings"
import { getSummary, updateContract, deleteContract } from "@/modules/contracts/lib/contract-repository"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import type { Category, CategoryFormData } from "@/types/category"
import type { Contract, ContractFormData } from "@/modules/contracts/types"
import { Button } from "@/components/ui/button"
import { CategoryCard } from "@/components/category-card"
import { CategoryDialog } from "@/components/category-dialog"
import { ContractsTable } from "@/modules/contracts/components/contracts-table"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { ImportDialog } from "@/modules/contracts/components/import-dialog"
import { getRenewalRowClass } from "@/lib/utils"

export function ContractsDashboardPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.contracts"), t("app.title"))
  const { data: categories = [] } = useCategories("contracts")
  const { data: summary } = useQuery({
    queryKey: ["summary"],
    queryFn: getSummary,
  })
  const summaryByCategory = new Map(
    (summary?.categories ?? []).map((s) => [s.id, s]),
  )
  const createCategory = useCreateCategory("contracts")
  const updateCategory = useUpdateCategory("contracts")
  const deleteCategory = useDeleteCategory("contracts")

  const { data: settings } = useSettings()
  const { data: upcomingContracts = [] } = useUpcomingRenewals(settings?.renewalDays)
  const qc = useQueryClient()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<Category | null>(null)
  const [deletingCategory, setDeletingCategory] = useState<Category | null>(null)
  const [editingContract, setEditingContract] = useState<Contract | null>(null)
  const [deletingContract, setDeletingContract] = useState<Contract | null>(null)

  const updateContractMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: ContractFormData }) => updateContract(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
      toast.success(t("contract.updated"))
      setEditingContract(null)
    },
  })

  const deleteContractMutation = useMutation({
    mutationFn: ({ id }: { id: string }) => deleteContract(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["contracts"] })
      qc.invalidateQueries({ queryKey: ["categories", "contracts"] })
      qc.invalidateQueries({ queryKey: ["summary"] })
      toast.success(t("contract.deleted"))
      setDeletingContract(null)
    },
  })

  function handleCreate(data: CategoryFormData) {
    createCategory.mutate(data, { onSuccess: () => toast.success(t("category.created")) })
  }

  function handleUpdate(data: CategoryFormData) {
    if (!editingCategory) return
    updateCategory.mutate(
      { id: editingCategory.id, data },
      { onSuccess: () => toast.success(t("category.updated")) },
    )
    setEditingCategory(null)
  }

  function handleDelete() {
    if (!deletingCategory) return
    deleteCategory.mutate(deletingCategory.id, {
      onSuccess: () => toast.success(t("category.deleted")),
    })
    setDeletingCategory(null)
  }

  function handleContractUpdate(data: ContractFormData) {
    if (!editingContract) return
    updateContractMutation.mutate({ id: editingContract.id, data })
  }

  function handleContractDelete() {
    if (!deletingContract) return
    deleteContractMutation.mutate({ id: deletingContract.id })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("nav.contracts")}</h1>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => setImportOpen(true)}>
            <Upload className="mr-2 h-4 w-4" />
            {t("import.button")}
          </Button>
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t("dashboard.newCategory")}
          </Button>
        </div>
      </div>

      {categories.length === 0 ? (
        <p className="text-muted-foreground">{t("dashboard.noCategories")}</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {categories.map((cat) => {
            const catSummary = summaryByCategory.get(cat.id)
            return (
              <CategoryCard
                key={cat.id}
                category={cat}
                module="contracts"
                totalAmount={catSummary?.monthlyTotal ?? 0}
                secondaryAmount={catSummary?.yearlyTotal ?? 0}
                itemLabel={t("dashboard.contractCount", { count: catSummary?.contractCount ?? 0 })}
                totalLabel={t("dashboard.monthlyTotal", {
                  amount: `${(catSummary?.monthlyTotal ?? 0).toFixed(2)} ${t("common.currency")}`,
                })}
                secondaryLabel={t("dashboard.yearlyTotal", {
                  amount: `${(catSummary?.yearlyTotal ?? 0).toFixed(2)} ${t("common.currency")}`,
                })}
                onEdit={() => setEditingCategory(cat)}
                onDelete={() => setDeletingCategory(cat)}
              />
            )
          })}
        </div>
      )}

      {upcomingContracts.length > 0 && (
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">{t("nav.upcomingRenewals")}</h2>
            <Link
              to="/contracts/upcoming-renewals"
              className="text-sm text-muted-foreground hover:text-foreground transition-colors"
            >
              {t("common.viewAll")}
            </Link>
          </div>
          <ContractsTable
            contracts={upcomingContracts.slice(0, 5)}
            onEdit={(c) => setEditingContract(c)}
            onDelete={(c) => setDeletingContract(c)}
            getRowClassName={getRenewalRowClass}
          />
        </div>
      )}

      <CategoryDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={handleCreate}
      />

      <CategoryDialog
        open={!!editingCategory}
        onOpenChange={(open) => { if (!open) setEditingCategory(null) }}
        category={editingCategory}
        onSubmit={handleUpdate}
      />

      <ImportDialog open={importOpen} onOpenChange={setImportOpen} />

      <DeleteConfirmDialog
        open={!!deletingCategory}
        onOpenChange={(open) => { if (!open) setDeletingCategory(null) }}
        description={t("category.deleteConfirm", { name: deletingCategory?.nameKey ? t(deletingCategory.nameKey) : deletingCategory?.name ?? "" })}
        onConfirm={handleDelete}
      />

      <ContractDialog
        open={!!editingContract}
        onOpenChange={(open) => { if (!open) setEditingContract(null) }}
        contract={editingContract}
        onSubmit={handleContractUpdate}
      />

      <DeleteConfirmDialog
        open={!!deletingContract}
        onOpenChange={(open) => { if (!open) setDeletingContract(null) }}
        description={t("contract.deleteConfirm", { name: deletingContract?.name ?? "" })}
        onConfirm={handleContractDelete}
      />
    </div>
  )
}
