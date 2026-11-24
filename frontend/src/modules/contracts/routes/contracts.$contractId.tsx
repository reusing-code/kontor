import { createRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { ItemDetailPage } from "@/components/category-items/item-detail-page"
import { contractItems } from "@/modules/contracts/config/category-items"

export const contractDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts/$contractId",
  beforeLoad: moduleGuard("contracts"),
  component: ContractDetailPage,
})

export function ContractDetailPage() {
  const { contractId } = contractDetailRoute.useParams()
  return <ItemDetailPage config={contractItems} itemId={contractId} />
}
