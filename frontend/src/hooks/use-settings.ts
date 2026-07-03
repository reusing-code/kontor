import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { updateSettings, changePassword } from "@/lib/settings-repository"
import { settingsQueryOptions } from "@/hooks/use-modules"
import type { SettingsUpdate } from "@/types/settings"

export function useSettings() {
  return useQuery(settingsQueryOptions)
}

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: SettingsUpdate) => updateSettings(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: settingsQueryOptions.queryKey }),
  })
}

export function useChangePassword() {
  return useMutation({
    mutationFn: ({ currentPassword, newPassword }: { currentPassword: string; newPassword: string }) =>
      changePassword(currentPassword, newPassword),
  })
}
