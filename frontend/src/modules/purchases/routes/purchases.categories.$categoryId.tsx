import { createRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { CategoryItemsPage } from "@/components/category-items/category-items-page"
import { purchaseItems } from "@/modules/purchases/config/category-items"

export const purchasesCategoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/purchases/categories/$categoryId",
  beforeLoad: moduleGuard("purchases"),
  component: PurchasesCategoryDetailPage,
})

function PurchasesCategoryDetailPage() {
  const { categoryId } = purchasesCategoryRoute.useParams()
  return <CategoryItemsPage config={purchaseItems} categoryId={categoryId} />
}
