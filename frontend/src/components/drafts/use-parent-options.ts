import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { useModules } from "@/hooks/use-modules"
import { getAllCategories } from "@/lib/category-repository"
import { getAllVehicles } from "@/modules/auto/lib/vehicle-repository"
import type { DraftEntityType } from "@/types/drafts"
import type { DraftOption } from "@/components/drafts/draft-input"

// useParentOptions lists the categories (purchase, contract) and vehicles
// (cost) of the enabled modules an entry can be created in.
export function useParentOptions(): Record<DraftEntityType, DraftOption[]> {
  const { t } = useTranslation()
  const { isEnabled } = useModules()
  const purchaseCategories = useQuery({
    queryKey: ["categories", "purchases"],
    queryFn: () => getAllCategories("purchases"),
    enabled: isEnabled("purchases"),
  })
  const contractCategories = useQuery({
    queryKey: ["categories", "contracts"],
    queryFn: () => getAllCategories("contracts"),
    enabled: isEnabled("contracts"),
  })
  const vehicles = useQuery({ queryKey: ["vehicles"], queryFn: getAllVehicles, enabled: isEnabled("auto") })

  return useMemo(() => {
    const categoryOptions = (cats: typeof purchaseCategories.data) =>
      (cats ?? []).map((c) => ({ id: c.id, label: c.nameKey ? t(c.nameKey) : c.name }))
    return {
      purchase: categoryOptions(purchaseCategories.data),
      contract: categoryOptions(contractCategories.data),
      cost: (vehicles.data ?? []).map((v) => ({ id: v.id, label: v.name })),
    }
  }, [purchaseCategories.data, contractCategories.data, vehicles.data, t])
}
