import type { Settings, SettingsUpdate } from "@/types/settings"
import { get, put } from "./api"

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
