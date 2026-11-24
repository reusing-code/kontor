import { useState } from "react"
import { formatMoney } from "@/lib/format"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus, Upload } from "lucide-react"
import { useCategories, useCreateCategory, useUpdateCategory, useDeleteCategory } from "@/hooks/use-categories"
import { useDeleteContract, useUpcomingRenewals, useUpdateContract } from "@/modules/contracts/hooks/use-contracts"
import { contractKeys } from "@/modules/contracts/keys"
import { useSettings } from "@/hooks/use-settings"
import { getSummary } from "@/modules/contracts/lib/contract-repository"
import { useQuery } from "@tanstack/react-query"
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
    queryKey: contractKeys.summary(),
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

  const [dialogOpen, setDialogOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<Category | null>(null)
  const [deletingCategory, setDeletingCategory] = useState<Category | null>(null)
  const [editingContract, setEditingContract] = useState<Contract | null>(null)
  const [deletingContract, setDeletingContract] = useState<Contract | null>(null)

  const updateContractMutation = useUpdateContract()

  const deleteContractMutation = useDeleteContract()

  function handleCreate(data: CategoryFormData) {
    return createCategory.mutateAsync(data, { onSuccess: () => toast.success(t("category.created")) })
  }

  function handleUpdate(data: CategoryFormData) {
    if (!editingCategory) return Promise.resolve()
    return updateCategory.mutateAsync(
      { id: editingCategory.id, data },
      { onSuccess: () => toast.success(t("category.updated")) },
    )
  }

  function handleDelete() {
    if (!deletingCategory) return
    deleteCategory.mutate(deletingCategory.id, {
      onSuccess: () => toast.success(t("category.deleted")),
    })
    setDeletingCategory(null)
  }

  function handleContractUpdate(data: ContractFormData) {
    if (!editingContract) return Promise.resolve()
    return updateContractMutation.mutateAsync({ id: editingContract.id, data }, { onSuccess: () => toast.success(t("contract.updated")) })
  }

  function handleContractDelete() {
    if (!deletingContract) return
    deleteContractMutation.mutate(deletingContract.id, {
      onSuccess: () => {
        toast.success(t("contract.deleted"))
        setDeletingContract(null)
      },
    })
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
                  amount: formatMoney(catSummary?.monthlyTotal ?? 0),
                })}
                secondaryLabel={t("dashboard.yearlyTotal", {
                  amount: formatMoney(catSummary?.yearlyTotal ?? 0),
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
