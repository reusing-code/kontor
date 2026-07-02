import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useCategories } from "@/hooks/use-categories"
import { cn } from "@/lib/utils"
import { SidebarSection } from "@/components/sidebar"

export function PurchasesSidebarSection() {
  const { t } = useTranslation()
  const { data: purchaseCategories = [] } = useCategories("purchases")
  const matchRoute = useMatchRoute()

  return (
    <SidebarSection
      title={t("nav.purchases")}
      to="/purchases"
      isActive={!!matchRoute({ to: "/purchases", fuzzy: true })}
    >
      {purchaseCategories.map((category) => {
        const active = matchRoute({
          to: "/purchases/categories/$categoryId",
          params: { categoryId: category.id },
        })
        return (
          <Link
            key={category.id}
            to="/purchases/categories/$categoryId"
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
