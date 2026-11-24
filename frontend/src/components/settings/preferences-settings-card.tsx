import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useSettings, type useUpdateSettings } from "@/hooks/use-settings"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

export function PreferencesSettingsCard({ updateSettings }: { updateSettings: ReturnType<typeof useUpdateSettings> }) {
  const { t } = useTranslation()
  const { data: settings } = useSettings()
  const [renewalDays, setRenewalDays] = useState<number | null>(null)
  const [reminderFrequency, setReminderFrequency] = useState<string | null>(null)

  const displayDays = renewalDays ?? settings?.renewalDays ?? 90
  const displayFrequency = reminderFrequency ?? settings?.reminderFrequency ?? "disabled"

  function handleSavePreferences(e: React.FormEvent) {
    e.preventDefault()
    updateSettings.mutate(
      { renewalDays: displayDays, reminderFrequency: displayFrequency },
      {
        onSuccess: () => toast.success(t("settings.saved")),
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("settings.preferences")}</CardTitle>
        <CardDescription>{t("settings.preferencesDescription")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSavePreferences} className="space-y-4">
          <div className="flex items-end gap-4">
            <div className="space-y-2">
              <Label htmlFor="renewalDays">{t("settings.renewalDays")}</Label>
              <Input
                id="renewalDays"
                type="number"
                min={1}
                max={365}
                value={displayDays}
                onChange={(e) => setRenewalDays(Number(e.target.value))}
                className="w-32"
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="reminderFrequency">{t("settings.reminderFrequency")}</Label>
            <Select value={displayFrequency} onValueChange={setReminderFrequency}>
              <SelectTrigger className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="disabled">{t("settings.reminderDisabled")}</SelectItem>
                <SelectItem value="weekly">{t("settings.reminderWeekly")}</SelectItem>
                <SelectItem value="biweekly">{t("settings.reminderBiweekly")}</SelectItem>
                <SelectItem value="monthly">{t("settings.reminderMonthly")}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button type="submit" disabled={updateSettings.isPending}>
            {t("common.save")}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
