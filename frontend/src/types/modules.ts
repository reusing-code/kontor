import type { LucideIcon } from "lucide-react"
import type { AnyRoute } from "@tanstack/react-router"
import type { ComponentType } from "react"

export type ModuleId = "contracts" | "purchases" | "auto" | "ledger"

export interface ModuleDefinition {
  id: ModuleId
  basePath: string
  labelKey: string
  icon: LucideIcon
  routes: AnyRoute[]
  SidebarSection: ComponentType
  HomeCard?: ComponentType
}
