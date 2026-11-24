import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { rootRoute } from "./__root"
import { useUpdateSettings } from "@/hooks/use-settings"
import { useModules } from "@/hooks/use-modules"
import { PaperlessSettingsCard } from "@/components/paperless/paperless-settings-card"
import { TokensSettingsCard } from "@/components/tokens/tokens-settings-card"
import { PreferencesSettingsCard } from "@/components/settings/preferences-settings-card"
import { PasswordSettingsCard } from "@/components/settings/password-settings-card"
import { ModulesSettingsCard } from "@/components/settings/modules-settings-card"
import { DataExportSettingsCard } from "@/components/settings/data-export-settings-card"

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

export function SettingsPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.settings"), t("app.title"))
  const { isEnabled } = useModules()
  // One mutation for both cards, so saving either disables the other.
  const updateSettings = useUpdateSettings()

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t("nav.settings")}</h1>

      {isEnabled("contracts") && <PreferencesSettingsCard updateSettings={updateSettings} />}

      <PasswordSettingsCard />

      <ModulesSettingsCard updateSettings={updateSettings} />

      <PaperlessSettingsCard />

      <TokensSettingsCard />

      <DataExportSettingsCard />
    </div>
  )
}
