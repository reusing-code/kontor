import type { Settings, SettingsUpdate } from "@/types/settings"
import type { ModuleId } from "@/types/modules"
import { get, post, put } from "./api"

export interface ImportResult {
  restored?: Record<string, number>
  warnings?: string[]
}

export async function getSettings(): Promise<Settings> {
  return get<Settings>("/settings")
}

export async function updateSettings(data: SettingsUpdate): Promise<Settings> {
  return put<Settings>("/settings", data)
}

// The change ends every session; the response carries the token of the new one.
export async function changePassword(currentPassword: string, newPassword: string): Promise<{ token: string }> {
  return put<{ token: string }>("/settings/password", { currentPassword, newPassword })
}

// Both read the export file in the browser; a file that is no JSON fails
// before anything is sent.
export async function restoreBackup(file: File): Promise<ImportResult> {
  return post<ImportResult>("/import", JSON.parse(await file.text()) as unknown)
}

export async function importModuleData(id: ModuleId, file: File): Promise<ImportResult> {
  return post<ImportResult>(`/modules/${id}/import`, JSON.parse(await file.text()) as unknown)
}
