import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { useLedgerCategories } from "@/hooks/use-ledger"
import { LedgerCategoryTreeManager } from "@/components/ledger-category-tree-manager"
import { rootRoute } from "./__root"

export const ledgerCategoriesRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger/categories",
  component: LedgerCategoriesPage,
})

function LedgerCategoriesPage() {
  const { t } = useTranslation()
  usePageTitle(t("ledger.categories"), t("app.title"))
  const { data: categories = [] } = useLedgerCategories()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("ledger.categories")}</h1>
        <p className="text-sm text-muted-foreground">{t("ledger.categoryTreeDescription")}</p>
      </div>
      <LedgerCategoryTreeManager categories={categories} />
    </div>
  )
}
