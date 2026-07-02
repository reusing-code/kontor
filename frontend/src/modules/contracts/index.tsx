import { FileText } from "lucide-react"
import type { ModuleDefinition } from "@/types/modules"
import { contractsRoutes } from "./routes/routes"
import { ContractsSidebarSection } from "./components/sidebar-section"
import { ContractsHomeCard } from "./components/home-card"

export const contractsModule: ModuleDefinition = {
  id: "contracts",
  basePath: "/contracts",
  labelKey: "nav.contracts",
  icon: FileText,
  routes: contractsRoutes,
  SidebarSection: ContractsSidebarSection,
  HomeCard: ContractsHomeCard,
}
