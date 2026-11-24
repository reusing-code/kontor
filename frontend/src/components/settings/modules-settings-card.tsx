import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useSettings, type useUpdateSettings } from "@/hooks/use-settings"
import { modules } from "@/modules/registry"
import type { ModuleId } from "@/types/modules"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Switch } from "@/components/ui/switch"

export function ModulesSettingsCard({ updateSettings }: { updateSettings: ReturnType<typeof useUpdateSettings> }) {
  const { t } = useTranslation()
  const { data: settings } = useSettings()

  function handleToggleModule(id: ModuleId, enabled: boolean) {
    if (!settings) return
    const enabledModules = enabled
      ? modules.map((m) => m.id).filter((moduleId) => moduleId === id || settings.enabledModules.includes(moduleId))
      : settings.enabledModules.filter((moduleId) => moduleId !== id)
    updateSettings.mutate(
      { renewalDays: settings.renewalDays, reminderFrequency: settings.reminderFrequency, enabledModules },
      {
        onSuccess: () => toast.success(t("settings.saved")),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.modules")}</CardTitle>
        <CardDescription>{t("settings.modulesDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {modules.map((m) => {
            const enabled = settings?.enabledModules.includes(m.id) ?? true
            return (
              <div key={m.id} className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                  <m.icon className="h-4 w-4 text-muted-foreground" />
                  <span className="text-sm font-medium">{t(m.labelKey)}</span>
                </div>
                <Switch
                  checked={enabled}
                  disabled={!settings || updateSettings.isPending}
                  onCheckedChange={(checked) => handleToggleModule(m.id, checked)}
                  aria-label={t(m.labelKey)}
                />
              </div>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}
