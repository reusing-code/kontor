import { Car } from "lucide-react"
import type { ModuleDefinition } from "@/types/modules"
import { autoRoutes } from "./routes/routes"
import { AutoSidebarSection } from "./components/sidebar-section"
import { AutoHomeCard } from "./components/home-card"

export const autoModule: ModuleDefinition = {
  id: "auto",
  basePath: "/auto",
  labelKey: "nav.auto",
  icon: Car,
  routes: autoRoutes,
  SidebarSection: AutoSidebarSection,
  HomeCard: AutoHomeCard,
}
