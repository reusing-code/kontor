import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { updateSettings, changePassword, importModuleData, restoreBackup } from "@/lib/settings-repository"
import { settingsQueryOptions } from "@/hooks/use-modules"
import i18n from "@/i18n"
import type { ModuleId } from "@/types/modules"
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

const importErrorMeta = { errorMessage: (error: Error) => error.message || i18n.t("settings.restoreFailed") }

// A restore or import changes settings and module data alike, so every query
// is refetched.
export function useRestoreBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => restoreBackup(file),
    onSuccess: () => qc.invalidateQueries(),
    meta: importErrorMeta,
  })
}

export function useImportModule(id: ModuleId) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => importModuleData(id, file),
    onSuccess: () => qc.invalidateQueries(),
    meta: importErrorMeta,
  })
}
