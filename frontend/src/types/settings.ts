import type { ModuleId } from "./modules"

export type Settings = {
  renewalDays: number
  reminderFrequency: string
  enabledModules: ModuleId[]
}

export type SettingsUpdate = {
  renewalDays: number
  reminderFrequency: string
  enabledModules?: ModuleId[]
}
