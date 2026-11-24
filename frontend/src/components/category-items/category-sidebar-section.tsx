import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useCategories } from "@/hooks/use-categories"
import { cn } from "@/lib/utils"
import { SidebarSection } from "@/components/sidebar"
import type { CategoryKind } from "@/types/category"

// The module's sidebar entry with one link per category.
export function CategorySidebarSection({ kind }: { kind: CategoryKind }) {
  const { t } = useTranslation()
  const { data: categories = [] } = useCategories(kind)
  const matchRoute = useMatchRoute()
  const categoryPath = `/${kind}/categories/$categoryId` as const

  return (
    <SidebarSection
      title={t(`nav.${kind}`)}
      to={`/${kind}`}
      isActive={!!matchRoute({ to: `/${kind}`, fuzzy: true })}
    >
      {categories.map((category) => {
        const active = matchRoute({ to: categoryPath, params: { categoryId: category.id } })
        return (
          <Link
            key={category.id}
            to={categoryPath}
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
