import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { useCategory } from "@/hooks/use-categories"
import { usePageTitle } from "@/hooks/use-page-title"
import { Button } from "@/components/ui/button"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import type { CategoryItem, CategoryItemConfig } from "@/components/category-items/types"

interface CategoryItemsPageProps<T extends CategoryItem, F> {
  config: CategoryItemConfig<T, F>
  categoryId: string
}

// The items of one category with create, edit and delete.
export function CategoryItemsPage<T extends CategoryItem, F>({ config, categoryId }: CategoryItemsPageProps<T, F>) {
  const { t } = useTranslation()
  const { entityType, hooks } = config
  const { data: category } = useCategory(config.kind, categoryId)
  const categoryName = category ? (category.nameKey ? t(category.nameKey) : category.name) : undefined
  usePageTitle(categoryName ?? t("nav.categories"), t("app.title"))
  const { data: items = [] } = hooks.useItemsByCategory(categoryId)
  const createItem = hooks.useCreateItem(categoryId)
  const updateItem = hooks.useUpdateItem()
  const deleteItem = hooks.useDeleteItem()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<T | null>(null)
  const [deleting, setDeleting] = useState<T | null>(null)

  function handleCreate(data: F) {
    return createItem.mutateAsync(data, { onSuccess: () => toast.success(t(`${entityType}.created`)) })
  }

  function handleUpdate(data: F) {
    if (!editing) return Promise.resolve()
    return updateItem.mutateAsync({ id: editing.id, data }, { onSuccess: () => toast.success(t(`${entityType}.updated`)) })
  }

  function handleDelete() {
    if (!deleting) return
    deleteItem.mutate(deleting.id, { onSuccess: () => toast.success(t(`${entityType}.deleted`)) })
    setDeleting(null)
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <h1 className="text-2xl font-bold">{categoryName ?? "..."}</h1>
        <div className="ml-auto">
          <Button onClick={() => setDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            {t(`${entityType}.create`)}
          </Button>
        </div>
      </div>

      {config.renderTable({ items, onEdit: setEditing, onDelete: setDeleting })}

      {config.renderDialog({ open: dialogOpen, onOpenChange: setDialogOpen, onSubmit: handleCreate })}

      {config.renderDialog({
        open: !!editing,
        onOpenChange: (open) => { if (!open) setEditing(null) },
        item: editing,
        onSubmit: handleUpdate,
      })}

      <DeleteConfirmDialog
        open={!!deleting}
        onOpenChange={(open) => { if (!open) setDeleting(null) }}
        description={t(`${entityType}.deleteConfirm`, { name: deleting ? config.itemName(deleting) : "" })}
        onConfirm={handleDelete}
      />
    </div>
  )
}
