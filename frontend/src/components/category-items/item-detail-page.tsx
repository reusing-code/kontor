import { useState } from "react"
import { formatDate, formatMoney } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { usePageTitle } from "@/hooks/use-page-title"
import { LinkedTransactionsList } from "@/components/linked-transactions-list"
import { PaperlessDocumentsSection } from "@/components/paperless/paperless-documents-section"
import { EmailSourceSection } from "@/components/email-source-section"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import type { CategoryItem, CategoryItemConfig, ItemDetailField } from "@/components/category-items/types"

interface ItemDetailPageProps<T extends CategoryItem, F> {
  config: CategoryItemConfig<T, F>
  itemId: string
}

function detailValue<T>(field: ItemDetailField<T>, item: T): string {
  const value = field.value(item)
  if (field.price) return typeof value === "number" && value ? formatMoney(value) : "-"
  if (field.date) return typeof value === "string" && value ? formatDate(value) : "-"
  return value ? String(value) : "-"
}

// One item with its fields, paperless documents, source email and linked
// transactions.
export function ItemDetailPage<T extends CategoryItem, F>({ config, itemId }: ItemDetailPageProps<T, F>) {
  const { t } = useTranslation()
  const { entityType, hooks } = config
  const { data: item } = hooks.useItem(itemId)
  const updateItem = hooks.useUpdateItem()
  const [editing, setEditing] = useState(false)

  usePageTitle(item ? config.itemName(item) : t(`nav.${config.kind}`), t("app.title"))

  if (!item) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">{config.itemName(item)}</h1>
        <Button variant="outline" onClick={() => setEditing(true)}>{t("common.edit")}</Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t(`${entityType}.edit`)}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          {config.detailFields.map((field) => (
            <div key={field.labelKey} className={cn(field.wide && "md:col-span-2")}>
              <div className="text-xs text-muted-foreground">{t(field.labelKey)}</div>
              <div className={cn(field.wide && "whitespace-pre-wrap")}>{detailValue(field, item)}</div>
            </div>
          ))}
        </CardContent>
      </Card>

      <PaperlessDocumentsSection
        entityType={entityType}
        entityId={item.id}
        entityUrl={`${window.location.origin}/${config.kind}/${item.id}`}
      />

      <EmailSourceSection entityType={entityType} entityId={item.id} />

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.linkedTransactions")}</CardTitle>
        </CardHeader>
        <CardContent>
          <LinkedTransactionsList transactionIds={item.linkedTransactionIds ?? []} />
        </CardContent>
      </Card>

      {config.renderDialog({
        open: editing,
        onOpenChange: setEditing,
        item,
        onSubmit: (data) =>
          updateItem.mutateAsync({ id: item.id, data }, { onSuccess: () => toast.success(t(`${entityType}.updated`)) }),
      })}
    </div>
  )
}
