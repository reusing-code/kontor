import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { useModules } from "@/hooks/use-modules"
import { categoriesQueryOptions } from "@/hooks/use-categories"
import { vehicleKeys } from "@/modules/auto/keys"
import { getAllVehicles } from "@/modules/auto/lib/vehicle-repository"
import { getLedgerCategories } from "@/modules/ledger/lib/ledger-repository"
import { ledgerKeys } from "@/modules/ledger/keys"
import type { LedgerCategory } from "@/modules/ledger/types"
import type { DraftEntityType } from "@/types/drafts"
import type { DraftOption } from "@/components/drafts/draft-input"

// ledgerCategoryOptions labels each category with its path ("Parent / Child").
function ledgerCategoryOptions(categories: LedgerCategory[]): DraftOption[] {
  const byId = new Map(categories.map((c) => [c.id, c]))
  const label = (c: LedgerCategory, depth = 0): string => {
    const parent = c.parentId && depth < 10 ? byId.get(c.parentId) : undefined
    return parent ? `${label(parent, depth + 1)} / ${c.name}` : c.name
  }
  return categories.map((c) => ({ id: c.id, label: label(c) })).sort((a, b) => a.label.localeCompare(b.label))
}

// useParentOptions lists what a draft can be filed under in the enabled
// modules: categories (purchase, contract), vehicles (cost) and the ledger
// category tree (optional parent of a ledger category). Types without a
// parent get an empty list.
export function useParentOptions(): Record<DraftEntityType, DraftOption[]> {
  const { t } = useTranslation()
  const { isEnabled } = useModules()
  const purchaseCategories = useQuery({ ...categoriesQueryOptions("purchases"), enabled: isEnabled("purchases") })
  const contractCategories = useQuery({ ...categoriesQueryOptions("contracts"), enabled: isEnabled("contracts") })
  const vehicles = useQuery({ queryKey: vehicleKeys.list(), queryFn: getAllVehicles, enabled: isEnabled("auto") })
  const ledgerCategories = useQuery({
    queryKey: ledgerKeys.categories(),
    queryFn: getLedgerCategories,
    enabled: isEnabled("ledger"),
  })

  return useMemo(() => {
    const categoryOptions = (cats: typeof purchaseCategories.data) =>
      (cats ?? []).map((c) => ({ id: c.id, label: c.nameKey ? t(c.nameKey) : c.name }))
    return {
      purchase: categoryOptions(purchaseCategories.data),
      contract: categoryOptions(contractCategories.data),
      cost: (vehicles.data ?? []).map((v) => ({ id: v.id, label: v.name })),
      vehicle: [],
      contract_category: [],
      purchase_category: [],
      ledger_category: ledgerCategoryOptions(ledgerCategories.data ?? []),
      ledger_account: [],
    }
  }, [purchaseCategories.data, contractCategories.data, vehicles.data, ledgerCategories.data, t])
}
