import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useImportModule } from "@/hooks/use-settings"
import type { ModuleId } from "@/types/modules"
import { download } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { showImportWarnings } from "@/components/settings/import-warnings"

// The backend imports a module only while it holds no entries, but swaps the
// module's categories for those in the file (replaceCategories); the auto
// module has none, so importing it replaces nothing and needs no confirmation.
const importReplacesCategories: ReadonlySet<ModuleId> = new Set(["contracts", "purchases", "ledger"])

export function ModuleDataRow({ id, label, Icon }: { id: ModuleId; label: string; Icon: React.ComponentType<{ className?: string }> }) {
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
        showImportWarnings(result)
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
