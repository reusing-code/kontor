import { createRoute } from "@tanstack/react-router"
import { rootRoute } from "@/routes/__root"
import { moduleGuard } from "@/modules/guard"
import { CategoryItemsPage } from "@/components/category-items/category-items-page"
import { contractItems } from "@/modules/contracts/config/category-items"

export const contractsCategoryRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts/categories/$categoryId",
  beforeLoad: moduleGuard("contracts"),
  component: ContractsCategoryDetailPage,
})

function ContractsCategoryDetailPage() {
  const { categoryId } = contractsCategoryRoute.useParams()
  return <CategoryItemsPage config={contractItems} categoryId={categoryId} />
}
