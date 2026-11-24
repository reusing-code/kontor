import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ChevronRight, ChevronsDownUp, ChevronsUpDown, Pencil, Plus, Trash2 } from "lucide-react"
import type { LedgerCategory, LedgerCategoryInput } from "@/modules/ledger/types"
import {
  useCreateLedgerCategory,
  useDeleteLedgerCategory,
  useUpdateLedgerCategory,
} from "@/modules/ledger/hooks/use-ledger"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
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
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(() => new Set())

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

  async function handleCreate(data: LedgerCategoryInput) {
    await createCategory.mutateAsync(data)
    toast.success(t("ledger.categoryCreated"))
    setDialogState(null)
  }

  async function handleUpdate(data: LedgerCategoryInput) {
    if (!dialogState?.category) return
    await updateCategory.mutateAsync({ id: dialogState.category.id, data })
    toast.success(t("ledger.categoryUpdated"))
    setDialogState(null)
  }

  function handleDelete() {
    if (!deleteCandidate) return
    deleteCategory.mutate(deleteCandidate.id, {
      onSuccess: () => {
        toast.success(t("ledger.categoryDeleted"))
        setDeleteCandidate(null)
      },
    })
  }

  const parentIds = useMemo(
    () => categories.filter((category) => childrenByParent.has(category.id)).map((category) => category.id),
    [categories, childrenByParent],
  )

  function toggle(id: string) {
    setCollapsed((current) => {
      const next = new Set(current)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }

  function renderTree(parentId?: string): React.ReactNode {
    const items = childrenByParent.get(parentId ?? "root") ?? []

    return items.map((category) => {
      const children = childrenByParent.get(category.id) ?? []
      const hasChildren = children.length > 0
      const expanded = hasChildren && !collapsed.has(category.id)
      const name = category.name

      return (
        <li key={category.id}>
          <div className="flex items-start gap-2 rounded-md px-2 py-1.5 hover:bg-muted/60">
            {hasChildren ? (
              <Button
                variant="ghost"
                size="icon-xs"
                className="mt-0.5 shrink-0 text-muted-foreground"
                aria-expanded={expanded}
                aria-label={t(expanded ? "ledger.categoryTree.collapse" : "ledger.categoryTree.expand", { name })}
                onClick={() => toggle(category.id)}
              >
                <ChevronRight className={cn("size-4 transition-transform", expanded && "rotate-90")} />
              </Button>
            ) : (
              <span aria-hidden="true" className="size-6 shrink-0" />
            )}

            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 py-0.5">
              <span className="font-medium">{name}</span>
              {hasChildren && !expanded ? (
                <span className="text-xs text-muted-foreground">
                  {t("ledger.categoryTree.childCount", { count: children.length })}
                </span>
              ) : null}
              {category.matchWords.length === 0 ? (
                <span className="text-xs text-muted-foreground">{t("ledger.noMatchWords")}</span>
              ) : (
                category.matchWords.map((word) => (
                  <Badge key={word} variant="outline" className="font-normal">{word}</Badge>
                ))
              )}
            </div>

            <div className="flex shrink-0 gap-0.5 text-muted-foreground">
              <Button
                variant="ghost"
                size="icon-sm"
                title={t("ledger.categoryTree.edit", { name })}
                aria-label={t("ledger.categoryTree.edit", { name })}
                onClick={() => setDialogState({ mode: "edit", category })}
              >
                <Pencil className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                title={t("ledger.categoryTree.addChild", { name })}
                aria-label={t("ledger.categoryTree.addChild", { name })}
                onClick={() => setDialogState({ mode: "create", category: null, parentId: category.id })}
              >
                <Plus className="size-4" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                className="hover:text-destructive"
                title={t("ledger.categoryTree.delete", { name })}
                aria-label={t("ledger.categoryTree.delete", { name })}
                onClick={() => setDeleteCandidate(category)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          </div>

          {expanded ? (
            <ul className="ml-5 border-l border-border pl-2">
              {renderTree(category.id)}
            </ul>
          ) : null}
        </li>
      )
    })
  }

  const dialogCategory = dialogState?.mode === "edit" ? dialogState.category : null

  return (
    <>
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-4">
          <CardTitle>{t("ledger.categories")}</CardTitle>
          <div className="flex flex-wrap gap-2">
            {parentIds.length > 0 ? (
              <>
                <Button variant="ghost" size="sm" onClick={() => setCollapsed(new Set())}>
                  <ChevronsUpDown className="size-4" />
                  {t("ledger.categoryTree.expandAll")}
                </Button>
                <Button variant="ghost" size="sm" onClick={() => setCollapsed(new Set(parentIds))}>
                  <ChevronsDownUp className="size-4" />
                  {t("ledger.categoryTree.collapseAll")}
                </Button>
              </>
            ) : null}
            <Button onClick={() => setDialogState({ mode: "create", category: null })}>
              <Plus className="size-4" />
              {t("ledger.newCategory")}
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {categories.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t("ledger.noCategories")}</p>
          ) : (
            <ul aria-label={t("ledger.categories")} className="-mx-2">
              {renderTree()}
            </ul>
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
        onSubmit={(data) => (dialogState?.mode === "edit" ? handleUpdate(data) : handleCreate(data))}
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
