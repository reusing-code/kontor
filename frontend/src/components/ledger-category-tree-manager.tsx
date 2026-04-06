import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Pencil, Plus, Trash2 } from "lucide-react"
import type { LedgerCategory, LedgerCategoryInput } from "@/types/ledger"
import {
  useCreateLedgerCategory,
  useDeleteLedgerCategory,
  useUpdateLedgerCategory,
} from "@/hooks/use-ledger"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { LedgerCategoryDialog } from "@/components/ledger-category-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

interface LedgerCategoryTreeManagerProps {
  categories: LedgerCategory[]
}

function sortCategories(categories: LedgerCategory[]) {
  return [...categories].sort((left, right) => left.name.localeCompare(right.name))
}

export function LedgerCategoryTreeManager({ categories }: LedgerCategoryTreeManagerProps) {
  const { t } = useTranslation()
  const createCategory = useCreateLedgerCategory()
  const updateCategory = useUpdateLedgerCategory()
  const deleteCategory = useDeleteLedgerCategory()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingCategory, setEditingCategory] = useState<LedgerCategory | null>(null)
  const [deleteCandidate, setDeleteCandidate] = useState<LedgerCategory | null>(null)

  const childrenByParent = useMemo(() => {
    const map = new Map<string, LedgerCategory[]>()
    for (const category of categories) {
      const key = category.parentId ?? "root"
      const existing = map.get(key) ?? []
      existing.push(category)
      map.set(key, existing)
    }
    for (const [key, items] of map) {
      map.set(key, sortCategories(items))
    }
    return map
  }, [categories])

  function handleCreate(data: LedgerCategoryInput) {
    createCategory.mutate(data, {
      onSuccess: () => {
        toast.success(t("ledger.categoryCreated"))
        setDialogOpen(false)
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleUpdate(data: LedgerCategoryInput) {
    if (!editingCategory) return
    updateCategory.mutate({ id: editingCategory.id, data }, {
      onSuccess: () => {
        toast.success(t("ledger.categoryUpdated"))
        setEditingCategory(null)
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleDelete() {
    if (!deleteCandidate) return
    deleteCategory.mutate(deleteCandidate.id, {
      onSuccess: () => {
        toast.success(t("ledger.categoryDeleted"))
        setDeleteCandidate(null)
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function renderTree(parentId?: string, depth: number = 0): React.ReactNode {
    const items = childrenByParent.get(parentId ?? "root") ?? []
    return items.map((category) => (
      <div key={category.id} className="space-y-3">
        <div className="rounded-lg border p-4" style={{ marginLeft: `${depth * 20}px` }}>
          <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
            <div className="space-y-2">
              <div className="font-medium">{category.name}</div>
              <div className="flex flex-wrap gap-2">
                {category.matchWords.length === 0 ? (
                  <span className="text-sm text-muted-foreground">{t("ledger.noMatchWords")}</span>
                ) : (
                  category.matchWords.map((word) => (
                    <Badge key={word} variant="outline">{word}</Badge>
                  ))
                )}
              </div>
            </div>
            <div className="flex gap-2">
              <Button size="sm" variant="outline" onClick={() => setEditingCategory(category)}>
                <Pencil className="h-4 w-4" />
                {t("common.edit")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => {
                setEditingCategory({
                  ...category,
                  id: "",
                } as LedgerCategory)
                setDialogOpen(true)
              }}>
                <Plus className="h-4 w-4" />
                {t("ledger.addChild")}
              </Button>
              <Button size="sm" variant="outline" onClick={() => setDeleteCandidate(category)}>
                <Trash2 className="h-4 w-4" />
                {t("common.delete")}
              </Button>
            </div>
          </div>
        </div>
        {renderTree(category.id, depth + 1)}
      </div>
    ))
  }

  const dialogCategory = dialogOpen && editingCategory?.id === "" ? null : editingCategory
  const dialogCategories = dialogOpen && editingCategory?.id === "" && editingCategory?.parentId
    ? categories
    : categories

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4">
          <CardTitle>{t("ledger.categories")}</CardTitle>
          <Button onClick={() => { setEditingCategory(null); setDialogOpen(true) }}>
            <Plus className="h-4 w-4" />
            {t("ledger.newCategory")}
          </Button>
        </CardHeader>
        <CardContent className="space-y-3">
          {categories.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("ledger.noCategories")}</p>
          ) : (
            renderTree()
          )}
        </CardContent>
      </Card>

      <LedgerCategoryDialog
        open={dialogOpen || !!editingCategory}
        onOpenChange={(open) => {
          if (!open) {
            setDialogOpen(false)
            setEditingCategory(null)
          }
        }}
        category={dialogCategory}
        categories={dialogCategories}
        onSubmit={(data) => {
          if (editingCategory && editingCategory.id) {
            handleUpdate(data)
            return
          }
          if (editingCategory && editingCategory.id === "") {
            handleCreate({ ...data, parentId: editingCategory.parentId })
            return
          }
          handleCreate(data)
        }}
      />

      <DeleteConfirmDialog
        open={!!deleteCandidate}
        onOpenChange={(open) => { if (!open) setDeleteCandidate(null) }}
        description={t("ledger.deleteCategoryConfirm", { name: deleteCandidate?.name ?? "" })}
        onConfirm={handleDelete}
      />
    </>
  )
}
