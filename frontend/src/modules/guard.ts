import { isRedirect, redirect } from "@tanstack/react-router"
import { queryClient } from "@/lib/query-client"
import { settingsQueryOptions } from "@/hooks/use-modules"
import type { ModuleId } from "@/types/modules"

export function moduleGuard(id: ModuleId) {
  return async () => {
    try {
      const settings = await queryClient.ensureQueryData(settingsQueryOptions)
      if (!settings.enabledModules.includes(id)) {
        throw redirect({ to: "/" })
      }
    } catch (error) {
      if (isRedirect(error)) {
        throw error
      }
    }
  }
}
