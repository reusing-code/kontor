import { useRef, useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { usePageTitle } from "@/hooks/use-page-title"
import { toast } from "sonner"
import { rootRoute } from "./__root"
import { useSettings, useUpdateSettings, useChangePassword, useImportModule, useRestoreBackup } from "@/hooks/use-settings"
import { useModules } from "@/hooks/use-modules"
import { modules } from "@/modules/registry"
import type { ModuleId } from "@/types/modules"
import { download } from "@/lib/api"
import type { ImportResult } from "@/lib/settings-repository"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { PaperlessSettingsCard } from "@/components/paperless/paperless-settings-card"
import { TokensSettingsCard } from "@/components/tokens/tokens-settings-card"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

// The backend imports a module only while it holds no entries, but swaps the
// module's categories for those in the file (replaceCategories); the auto
// module has none, so importing it replaces nothing and needs no confirmation.
const importReplacesCategories: ReadonlySet<ModuleId> = new Set(["contracts", "purchases", "ledger"])

function showWarnings(result: ImportResult) {
  for (const warning of result.warnings ?? []) {
    toast.warning(warning)
  }
}

export function SettingsPage() {
  const { t } = useTranslation()
  usePageTitle(t("nav.settings"), t("app.title"))
  const { data: settings } = useSettings()
  const { isEnabled } = useModules()
  const updateSettings = useUpdateSettings()
  const changePassword = useChangePassword()
  const restoreBackup = useRestoreBackup()

  const [renewalDays, setRenewalDays] = useState<number | null>(null)
  const [reminderFrequency, setReminderFrequency] = useState<string | null>(null)
  const [currentPassword, setCurrentPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [exporting, setExporting] = useState(false)
  // The picked file waits here until the user confirmed the restore.
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const restoreInputRef = useRef<HTMLInputElement>(null)

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
      },
    )
  }

  async function handleExport() {
    setExporting(true)
    try {
      await download("/export", "kontor-export.json")
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("settings.exportFailed"))
    } finally {
      setExporting(false)
    }
  }

  function handleRestoreFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (file) setRestoreFile(file)
  }

  function handleRestore() {
    if (!restoreFile) return
    restoreBackup.mutate(restoreFile, {
      onSuccess: (result) => {
        toast.success(t("settings.restoreSucceeded"))
        showWarnings(result)
      },
    })
    setRestoreFile(null)
  }

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold">{t("nav.settings")}</h1>

      {isEnabled("contracts") && (
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
      )}

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

      <PaperlessSettingsCard />

      <TokensSettingsCard />

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
              disabled={restoreBackup.isPending}
              variant="outline"
            >
              {restoreBackup.isPending ? t("settings.restoring") : t("settings.restoreButton")}
            </Button>
            <input
              ref={restoreInputRef}
              type="file"
              accept="application/json,.json"
              className="hidden"
              aria-label={t("settings.restoreButton")}
              onChange={handleRestoreFile}
            />
          </div>
          <DeleteConfirmDialog
            open={restoreFile !== null}
            onOpenChange={(open) => { if (!open) setRestoreFile(null) }}
            description={t("settings.restoreConfirm", { file: restoreFile?.name ?? "" })}
            actionLabel={t("settings.restoreAction")}
            onConfirm={handleRestore}
          />
          <p className="text-sm text-muted-foreground">{t("settings.restoreHint")}</p>

          <div className="space-y-3 border-t pt-4">
            <p className="text-sm font-medium">{t("settings.moduleData")}</p>
            {modules
              .filter((m) => isEnabled(m.id))
              .map((m) => (
                <ModuleDataRow key={m.id} id={m.id} label={t(m.labelKey)} Icon={m.icon} />
              ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function ModuleDataRow({ id, label, Icon }: { id: ModuleId; label: string; Icon: React.ComponentType<{ className?: string }> }) {
  const { t } = useTranslation()
  const [exporting, setExporting] = useState(false)
  const importModule = useImportModule(id)
  const [importFile, setImportFile] = useState<File | null>(null)
  const importInputRef = useRef<HTMLInputElement>(null)

  async function handleExport() {
    setExporting(true)
    try {
      await download(`/modules/${id}/export`, `kontor-export-${id}.json`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("settings.exportFailed"))
    } finally {
      setExporting(false)
    }
  }

  function handleImportFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ""
    if (!file) return
    if (importReplacesCategories.has(id)) setImportFile(file)
    else runImport(file)
  }

  function runImport(file: File) {
    importModule.mutate(file, {
      onSuccess: (result) => {
        toast.success(t("settings.moduleImportSucceeded", { module: label }))
        showWarnings(result)
      },
    })
    setImportFile(null)
  }

  function handleImport() {
    if (importFile) runImport(importFile)
  }

  return (
    <div className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-3">
        <Icon className="h-4 w-4 text-muted-foreground" />
        <span className="text-sm">{label}</span>
      </div>
      <div className="flex gap-2">
        <Button onClick={handleExport} disabled={exporting} variant="outline" size="sm">
          {t("settings.moduleExportButton")}
        </Button>
        <Button onClick={() => importInputRef.current?.click()} disabled={importModule.isPending} variant="outline" size="sm">
          {t("settings.moduleImportButton")}
        </Button>
        <input
          ref={importInputRef}
          type="file"
          accept="application/json,.json"
          className="hidden"
          aria-label={t("settings.moduleImportFile", { module: label })}
          onChange={handleImportFile}
        />
      </div>
      <DeleteConfirmDialog
        open={importFile !== null}
        onOpenChange={(open) => { if (!open) setImportFile(null) }}
        description={t("settings.moduleImportConfirm", { module: label, file: importFile?.name ?? "" })}
        actionLabel={t("settings.moduleImportButton")}
        onConfirm={handleImport}
      />
    </div>
  )
}
