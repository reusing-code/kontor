import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { usePageTitle } from "@/hooks/use-page-title"
import { usePurchase, useUpdatePurchaseById } from "@/hooks/use-purchases"
import { LinkedTransactionsList } from "@/components/linked-transactions-list"
import { PurchaseDialog } from "@/components/purchase-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { rootRoute } from "./__root"

export const purchaseDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases/$purchaseId",
  component: PurchaseDetailPage,
})

export function PurchaseDetailPage() {
  const { t } = useTranslation()
  const { purchaseId } = purchaseDetailRoute.useParams()
  const { data: purchase } = usePurchase(purchaseId)
  const updatePurchase = useUpdatePurchaseById()
  const [editing, setEditing] = useState(false)

  usePageTitle(purchase?.itemName ?? t("nav.purchases"), t("app.title"))

  if (!purchase) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">{purchase.itemName}</h1>
        <Button variant="outline" onClick={() => setEditing(true)}>{t("common.edit")}</Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("purchase.edit")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div><div className="text-xs text-muted-foreground">{t("purchaseFields.type")}</div><div>{purchase.type || "-"}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("purchaseFields.dealer")}</div><div>{purchase.dealer || "-"}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("purchaseFields.price")}</div><div>{purchase.price ? `${purchase.price.toFixed(2)} ${t("common.currency")}` : "-"}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("purchaseFields.purchaseDate")}</div><div>{purchase.purchaseDate || "-"}</div></div>
          <div className="md:col-span-2"><div className="text-xs text-muted-foreground">{t("purchaseFields.comments")}</div><div className="whitespace-pre-wrap">{purchase.comments || "-"}</div></div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.linkedTransactions")}</CardTitle>
        </CardHeader>
        <CardContent>
          <LinkedTransactionsList transactionIds={purchase.linkedTransactionIds ?? []} />
        </CardContent>
      </Card>

      <PurchaseDialog
        open={editing}
        onOpenChange={setEditing}
        purchase={purchase}
        onSubmit={(data) => updatePurchase.mutate({ id: purchase.id, data }, { onSuccess: () => toast.success(t("purchase.updated")) })}
      />
    </div>
  )
}
