import { Landmark } from "lucide-react"
import type { ModuleDefinition } from "@/types/modules"
import { ledgerRoutes } from "./routes/routes"
import { LedgerSidebarSection } from "./components/sidebar-section"
import { LedgerHomeCard } from "./components/home-card"

export const ledgerModule: ModuleDefinition = {
  id: "ledger",
  basePath: "/ledger",
  labelKey: "nav.ledger",
  icon: Landmark,
  routes: ledgerRoutes,
  SidebarSection: LedgerSidebarSection,
  HomeCard: LedgerHomeCard,
}
