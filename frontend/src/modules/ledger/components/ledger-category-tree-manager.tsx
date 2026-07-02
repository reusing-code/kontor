import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ChevronRight, Pencil, Plus, Trash2 } from "lucide-react"
import type { LedgerCategory, LedgerCategoryInput } from "@/modules/ledger/types"
import {
  useCreateLedgerCategory,
  useDeleteLedgerCategory,
  useUpdateLedgerCategory,
} from "@/modules/ledger/hooks/use-ledger"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { LedgerCategoryDialog } from "@/modules/ledger/components/ledger-category-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

interface LedgerCategoryTreeManagerProps {
  categories: LedgerCategory[]
}

interface CategoryDialogState {
  mode: "create" | "edit"
  category: LedgerCategory | null
  parentId?: string
}

function sortCategories(categories: LedgerCategory[]) {
  return [...categories].sort((left, right) => left.name.localeCompare(right.name))
}

export function LedgerCategoryTreeManager({ categories }: LedgerCategoryTreeManagerProps) {
  const { t } = useTranslation()
  const createCategory = useCreateLedgerCategory()
  const updateCategory = useUpdateLedgerCategory()
  const deleteCategory = useDeleteLedgerCategory()
  const [dialogState, setDialogState] = useState<CategoryDialogState | null>(null)
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
        setDialogState(null)
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleUpdate(data: LedgerCategoryInput) {
    if (!dialogState?.category) return
    updateCategory.mutate({ id: dialogState.category.id, data }, {
      onSuccess: () => {
        toast.success(t("ledger.categoryUpdated"))
        setDialogState(null)
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

  function renderTree(parentId: string | undefined = undefined, depth: number = 0): React.ReactNode {
    const items = childrenByParent.get(parentId ?? "root") ?? []

    return items.map((category) => {
      const children = childrenByParent.get(category.id) ?? []

      return (
        <div key={category.id} className="space-y-3">
          <div className="relative rounded-lg border bg-card/60 p-4">
            {depth > 0 ? (
              <div aria-hidden="true" className="pointer-events-none absolute left-0 top-0 bottom-0 w-8">
                <div className="absolute left-4 top-0 bottom-6 border-l border-border" />
                <div className="absolute left-4 top-6 w-4 border-t border-border" />
              </div>
            ) : null}

            <div className="flex flex-col gap-3" style={{ paddingLeft: depth > 0 ? `${depth * 20}px` : undefined }}>
              <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                <div className="space-y-2">
                  <div className="flex items-center gap-2">
                    {children.length > 0 ? <ChevronRight className="h-4 w-4 text-muted-foreground" /> : <span className="w-4" />}
                    <span className="font-medium">{category.name}</span>
                  </div>
                  <div className="flex flex-wrap gap-2 pl-6">
                    {category.matchWords.length === 0 ? (
                      <span className="text-sm text-muted-foreground">{t("ledger.noMatchWords")}</span>
                    ) : (
                      category.matchWords.map((word) => (
                        <Badge key={word} variant="outline">{word}</Badge>
                      ))
                    )}
                  </div>
                </div>

                <div className="flex gap-2 pl-6 md:pl-0">
                  <Button size="sm" variant="outline" onClick={() => setDialogState({ mode: "edit", category })}>
                    <Pencil className="h-4 w-4" />
                    {t("common.edit")}
                  </Button>
                  <Button size="sm" variant="outline" onClick={() => setDialogState({ mode: "create", category: null, parentId: category.id })}>
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
          </div>

          {children.length > 0 ? <div className="space-y-3">{renderTree(category.id, depth + 1)}</div> : null}
        </div>
      )
    })
  }

  const dialogCategory = dialogState?.mode === "edit" ? dialogState.category : null

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-4">
          <CardTitle>{t("ledger.categories")}</CardTitle>
          <Button onClick={() => setDialogState({ mode: "create", category: null })}>
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
        open={dialogState !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDialogState(null)
          }
        }}
        category={dialogCategory}
        categories={categories}
        initialParentId={dialogState?.mode === "create" ? dialogState.parentId : undefined}
        onSubmit={(data) => {
          if (dialogState?.mode === "edit") {
            handleUpdate(data)
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
