import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useCategories } from "@/hooks/use-categories"
import { cn } from "@/lib/utils"
import { SidebarSection } from "@/components/sidebar"

export function ContractsSidebarSection() {
  const { t } = useTranslation()
  const { data: contractCategories = [] } = useCategories("contracts")
  const matchRoute = useMatchRoute()

  return (
    <SidebarSection
      title={t("nav.contracts")}
      to="/contracts"
      isActive={!!matchRoute({ to: "/contracts", fuzzy: true })}
    >
      {contractCategories.map((category) => {
        const active = matchRoute({
          to: "/contracts/categories/$categoryId",
          params: { categoryId: category.id },
        })
        return (
          <Link
            key={category.id}
            to="/contracts/categories/$categoryId"
            params={{ categoryId: category.id }}
            className={cn(
              "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
              active && "bg-accent font-medium",
            )}
          >
            {category.nameKey ? t(category.nameKey) : category.name}
          </Link>
        )
      })}
    </SidebarSection>
  )
}
