import { queryOptions, useQuery } from "@tanstack/react-query"
import { getSettings } from "@/lib/settings-repository"
import type { ModuleId } from "@/types/modules"

const ALL_MODULE_IDS: ModuleId[] = ["contracts", "purchases", "auto", "ledger"]

export const settingsQueryOptions = queryOptions({
  queryKey: ["settings"],
  queryFn: getSettings,
})

export function useModules() {
  const { data, isLoading } = useQuery(settingsQueryOptions)
  const enabledModules = data?.enabledModules ?? ALL_MODULE_IDS

  return {
    enabledModules,
    isLoading,
    isEnabled: (id: ModuleId) => enabledModules.includes(id),
  }
}
