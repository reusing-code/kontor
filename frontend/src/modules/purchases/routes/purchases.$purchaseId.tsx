import { createRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { ItemDetailPage } from "@/components/category-items/item-detail-page"
import { purchaseItems } from "@/modules/purchases/config/category-items"

export const purchaseDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases/$purchaseId",
  beforeLoad: moduleGuard("purchases"),
  component: PurchaseDetailPage,
})

export function PurchaseDetailPage() {
  const { purchaseId } = purchaseDetailRoute.useParams()
  return <ItemDetailPage config={purchaseItems} itemId={purchaseId} />
}
