import { cn } from "@/lib/utils"

export function sidebarLinkClass(active: boolean): string {
  return cn("rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent", active && "bg-accent font-medium")
}
