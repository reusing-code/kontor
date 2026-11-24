import type { Settings, SettingsUpdate } from "@/types/settings"
import { get, put, sendJson } from "./api"

export async function getSettings(): Promise<Settings> {
  return get<Settings>("/settings")
}

export async function updateSettings(data: SettingsUpdate): Promise<Settings> {
  return put<Settings>("/settings", data)
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  // A wrong current password answers 401 (#52), which must not end the session.
  return sendJson<void>("PUT", "/settings/password", {
    json: { currentPassword, newPassword },
    redirectOnUnauthorized: false,
  })
}
