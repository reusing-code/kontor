import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useRestoreBackup } from "@/hooks/use-settings"
import { useModules } from "@/hooks/use-modules"
import { modules } from "@/modules/registry"
import { download } from "@/lib/api"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { ModuleDataRow } from "@/components/settings/module-data-row"
import { showImportWarnings } from "@/components/settings/import-warnings"

export function DataExportSettingsCard() {
  const { t } = useTranslation()
  const { isEnabled } = useModules()
  const restoreBackup = useRestoreBackup()
  const [exporting, setExporting] = useState(false)
  // The picked file waits here until the user confirmed the restore.
  const [restoreFile, setRestoreFile] = useState<File | null>(null)
  const restoreInputRef = useRef<HTMLInputElement>(null)

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
        showImportWarnings(result)
      },
    })
    setRestoreFile(null)
  }

  return (
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
  )
}
