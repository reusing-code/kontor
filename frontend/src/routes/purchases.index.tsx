import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { rootRoute } from "./__root"
import { useCategories, useCreateCategory, useUpdateCategory, useDeleteCategory } from "@/hooks/use-categories"
import { getPurchaseSummary } from "@/lib/purchase-repository"
import { useQuery } from "@tanstack/react-query"
import type { Category, CategoryFormData } from "@/types/category"
import { Button } from "@/components/ui/button"
import { CategoryCard } from "@/components/category-card"
import { CategoryDialog } from "@/components/category-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

export const purchasesIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases",
  component: PurchasesDashboardPage,
})

export function PurchasesDashboardPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.purchases"), t("app.title"))
  const { data: categories = [] } = useCategories("purchases")
  const { data: summary } = useQuery({
    queryKey: ["purchases-summary"],
    queryFn: getPurchaseSummary,
  })
  const summaryByCategory = new Map(
    (summary?.categories ?? []).map((s) => [s.id, s]),
  )
  const createCategory = useCreateCategory("purchases")
  const updateCategory = useUpdateCategory("purchases")
  const deleteCategory = useDeleteCategory("purchases")

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<Category | null>(null)
  const [deletingCategory, setDeletingCategory] = useState<Category | null>(null)

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

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">{t("nav.purchases")}</h1>
        <Button onClick={() => setDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          {t("dashboard.newCategory")}
        </Button>
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
                module="purchases"
                totalAmount={catSummary?.totalSpent ?? 0}
                itemLabel={t("purchase.purchaseCount", { count: catSummary?.purchaseCount ?? 0 })}
                totalLabel={t("purchase.totalSpent", {
                  amount: `${(catSummary?.totalSpent ?? 0).toFixed(2)} ${t("common.currency")}`,
                })}
                onEdit={() => setEditingCategory(cat)}
                onDelete={() => setDeletingCategory(cat)}
              />
            )
          })}
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

      <DeleteConfirmDialog
        open={!!deletingCategory}
        onOpenChange={(open) => { if (!open) setDeletingCategory(null) }}
        description={t("category.deleteConfirm", { name: deletingCategory?.nameKey ? t(deletingCategory.nameKey) : deletingCategory?.name ?? "" })}
        onConfirm={handleDelete}
      />
    </div>
  )
}
