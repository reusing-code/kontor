import { ShoppingBag } from "lucide-react"
import type { ModuleDefinition } from "@/types/modules"
import { purchasesRoutes } from "./routes/routes"
import { PurchasesSidebarSection } from "./components/sidebar-section"
import { PurchasesHomeCard } from "./components/home-card"

export const purchasesModule: ModuleDefinition = {
  id: "purchases",
  basePath: "/purchases",
  labelKey: "nav.purchases",
  icon: ShoppingBag,
  routes: purchasesRoutes,
  SidebarSection: PurchasesSidebarSection,
  HomeCard: PurchasesHomeCard,
}
