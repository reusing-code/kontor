import type { ReactNode } from "react"
import type { CategoryItemHooks } from "@/hooks/category-item-hooks"
import type { CategoryKind } from "@/types/category"

export interface CategoryItem {
  id: string
  linkedTransactionIds?: string[]
}

export interface ItemTableProps<T> {
  items: T[]
  onEdit: (item: T) => void
  onDelete: (item: T) => void
}

export interface ItemDialogProps<T, F> {
  open: boolean
  onOpenChange: (open: boolean) => void
  item?: T | null
  onSubmit: (data: F) => Promise<unknown>
}

export interface ItemDetailField<T> {
  labelKey: string
  value: (item: T) => string | number | undefined
  price?: boolean
  date?: boolean
  // Spans the card and keeps line breaks.
  wide?: boolean
}

// What the shared category and detail pages need to know about a module.
export interface CategoryItemConfig<T extends CategoryItem, F> {
  kind: CategoryKind
  // The entity type of paperless and email links, also the i18n prefix
  // ("contract.created", "purchase.deleteConfirm", ...).
  entityType: "contract" | "purchase"
  itemName: (item: T) => string
  hooks: CategoryItemHooks<T, F>
  detailFields: ItemDetailField<T>[]
  renderTable: (props: ItemTableProps<T>) => ReactNode
  renderDialog: (props: ItemDialogProps<T, F>) => ReactNode
}
