import { useRef, useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { rootRoute } from "./__root"
import { useSettings, useUpdateSettings, useChangePassword } from "@/hooks/use-settings"
import { download, post } from "@/lib/api"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

export function SettingsPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.settings"), t("app.title"))
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  const changePassword = useChangePassword()

  const [renewalDays, setRenewalDays] = useState<number | null>(null)
  const [reminderFrequency, setReminderFrequency] = useState<string | null>(null)
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [exporting, setExporting] = useState(false)
  const [restoring, setRestoring] = useState(false)
  const restoreInputRef = useRef<HTMLInputElement>(null)

  const displayDays = renewalDays ?? settings?.renewalDays ?? 90
  const displayFrequency = reminderFrequency ?? settings?.reminderFrequency ?? "disabled"

  function handleSavePreferences(e: React.FormEvent) {
    e.preventDefault()
    updateSettings.mutate(
      { renewalDays: displayDays, reminderFrequency: displayFrequency },
      {
        onSuccess: () => toast.success(t("settings.saved")),
        onError: () => toast.error(t("settings.saveFailed")),
      },
    )
  }

  function handleChangePassword(e: React.FormEvent) {
    e.preventDefault()
    if (newPassword.length < 8) {
      toast.error(t("auth.passwordTooShort"))
      return
    }
    if (newPassword !== confirmPassword) {
      toast.error(t("auth.passwordMismatch"))
      return
    }
    changePassword.mutate(
      { currentPassword, newPassword },
      {
        onSuccess: () => {
          toast.success(t("settings.passwordChanged"))
          setCurrentPassword("")
          setNewPassword("")
          setConfirmPassword("")
        },
        onError: (err) => {
          toast.error(err.message || t("settings.passwordChangeFailed"))
        },
      },
    )
  }

  async function handleExport() {
    setExporting(true)
    try {
      await download("/export", "contracts-export.json")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("settings.exportFailed"))
    } finally {
      setExporting(false)
    }
  }

  async function handleRestoreFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file) return
    setRestoring(true)
    try {
      const payload: unknown = JSON.parse(await file.text())
      const result = await post<{ warnings: string[] }>("/restore", payload)
      toast.success(t("settings.restoreSucceeded"))
      for (const warning of result.warnings ?? []) {
        toast.warning(warning)
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("settings.restoreFailed"))
    } finally {
      setRestoring(false)
    }
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t("nav.settings")}</h1>

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

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.changePassword")}</CardTitle>
          <CardDescription>{t("settings.changePasswordDescription")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleChangePassword} className="space-y-4 max-w-sm">
            <div className="space-y-2">
              <Label htmlFor="currentPassword">{t("settings.currentPassword")}</Label>
              <Input
                id="currentPassword"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="newPassword">{t("settings.newPassword")}</Label>
              <Input
                id="newPassword"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirmPassword">{t("auth.confirmPassword")}</Label>
              <Input
                id="confirmPassword"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                required
              />
            </div>
            <Button type="submit" disabled={changePassword.isPending}>
              {t("settings.changePassword")}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("settings.dataExport")}</CardTitle>
          <CardDescription>{t("settings.dataExportDescription")}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex gap-3">
            <Button onClick={handleExport} disabled={exporting} variant="outline">
              {exporting ? t("settings.exporting") : t("settings.exportButton")}
            </Button>
            <Button
              onClick={() => restoreInputRef.current?.click()}
              disabled={restoring}
              variant="outline"
            >
              {restoring ? t("settings.restoring") : t("settings.restoreButton")}
            </Button>
            <input
              ref={restoreInputRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              onChange={handleRestoreFile}
            />
          </div>
          <p className="text-sm text-muted-foreground">{t("settings.restoreHint")}</p>
        </CardContent>
      </Card>
    </div>
  )
}
