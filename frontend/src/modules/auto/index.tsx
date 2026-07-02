import { Car } from "lucide-react"
import type { ModuleDefinition } from "@/types/modules"
import { autoRoutes } from "./routes/routes"
import { AutoSidebarSection } from "./components/sidebar-section"

export const autoModule: ModuleDefinition = {
  id: "auto",
  basePath: "/auto",
  labelKey: "nav.auto",
  icon: Car,
  routes: autoRoutes,
  SidebarSection: AutoSidebarSection,
}
