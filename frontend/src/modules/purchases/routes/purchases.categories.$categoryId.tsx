import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { useCategoryPurchases, useCreatePurchase, useUpdatePurchase, useDeletePurchase } from "@/modules/purchases/hooks/use-purchases"
import { getCategoryById } from "@/lib/category-repository"
import { useQuery } from "@tanstack/react-query"
import type { Purchase, PurchaseFormData } from "@/modules/purchases/types"
import { Button } from "@/components/ui/button"
import { PurchasesTable } from "@/modules/purchases/components/purchases-table"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

export const purchasesCategoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases/categories/$categoryId",
  beforeLoad: moduleGuard("purchases"),
  component: PurchasesCategoryDetailPage,
})

function PurchasesCategoryDetailPage() {
  const { t } = useTranslation()
  const { categoryId } = purchasesCategoryRoute.useParams()
  const { data: category } = useQuery({
    queryKey: ["category", "purchases", categoryId],
    queryFn: () => getCategoryById("purchases", categoryId),
  })
  const categoryName = category ? (category.nameKey ? t(category.nameKey) : category.name) : t("nav.categories")
  usePageTitle(categoryName, t("app.title"))
  const { data: purchases = [] } = useCategoryPurchases(categoryId)
  const createPurchase = useCreatePurchase(categoryId)
  const updatePurchase = useUpdatePurchase(categoryId)
  const deletePurchase = useDeletePurchase(categoryId)

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingPurchase, setEditingPurchase] = useState<Purchase | null>(null)
  const [deletingPurchase, setDeletingPurchase] = useState<Purchase | null>(null)

  function handleCreate(data: PurchaseFormData) {
    createPurchase.mutate(data, { onSuccess: () => toast.success(t("purchase.created")) })
  }

  function handleUpdate(data: PurchaseFormData) {
    if (!editingPurchase) return
    updatePurchase.mutate(
      { id: editingPurchase.id, data },
      { onSuccess: () => toast.success(t("purchase.updated")) },
    )
    setEditingPurchase(null)
  }

  function handleDelete() {
    if (!deletingPurchase) return
    deletePurchase.mutate(deletingPurchase.id, {
      onSuccess: () => toast.success(t("purchase.deleted")),
    })
    setDeletingPurchase(null)
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
            {t("purchase.create")}
          </Button>
        </div>
      </div>

      <PurchasesTable
        purchases={purchases}
        onEdit={(p) => setEditingPurchase(p)}
        onDelete={(p) => setDeletingPurchase(p)}
      />

      <PurchaseDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSubmit={handleCreate}
      />

      <PurchaseDialog
        open={!!editingPurchase}
        onOpenChange={(open) => { if (!open) setEditingPurchase(null) }}
        purchase={editingPurchase}
        onSubmit={handleUpdate}
      />

      <DeleteConfirmDialog
        open={!!deletingPurchase}
        onOpenChange={(open) => { if (!open) setDeletingPurchase(null) }}
        description={t("purchase.deleteConfirm", { name: deletingPurchase?.itemName ?? "" })}
        onConfirm={handleDelete}
      />
    </div>
  )
}
