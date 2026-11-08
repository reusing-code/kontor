import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  usePaperlessPipelineSettings,
  useRunPaperlessPipeline,
  useUpdatePaperlessPipelineSettings,
} from "@/hooks/use-paperless"
import type { PaperlessPipelineSettings } from "@/types/paperless"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Separator } from "@/components/ui/separator"

// Rendered by PaperlessSettingsCard only when the LLM is available and
// paperless is configured.
export function PaperlessPipelineSection() {
  const { t } = useTranslation()
  const { data: settings } = usePaperlessPipelineSettings(true)
  const updateSettings = useUpdatePaperlessPipelineSettings()
  const runPipeline = useRunPaperlessPipeline()
  const [edits, setEdits] = useState<Partial<PaperlessPipelineSettings>>({})

  if (!settings) return null
  const current: PaperlessPipelineSettings = { ...settings, ...edits }

  function save(next: PaperlessPipelineSettings) {
    updateSettings.mutate(next, {
      onSuccess: () => {
        setEdits({})
        toast.success(t("paperless.pipeline.saved"))
      },
      onError: (err) => toast.error(err.message || t("settings.saveFailed")),
    })
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    save(current)
  }

  function handleRun() {
    runPipeline.mutate(undefined, {
      onSuccess: (res) => {
        toast.success(t("paperless.pipeline.runResult", { count: res.created, found: res.found }))
        if (res.failed > 0) toast.warning(t("paperless.pipeline.runFailed", { count: res.failed }))
      },
      onError: (err) => toast.error(err.message),
    })
  }

  return (
    <div className="space-y-4 max-w-xl">
      <Separator />
      <div className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium">{t("paperless.pipeline.title")}</p>
          <p className="text-sm text-muted-foreground">
            {t("paperless.pipeline.description", { inboxTag: settings.inboxTag, doneTag: settings.doneTag })}
          </p>
        </div>
        <Switch
          checked={settings.enabled}
          disabled={updateSettings.isPending}
          onCheckedChange={(enabled) => save({ ...settings, enabled })}
          aria-label={t("paperless.pipeline.enable")}
        />
      </div>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="paperlessInboxTag">{t("paperless.pipeline.inboxTag")}</Label>
            <Input
              id="paperlessInboxTag"
              value={current.inboxTag}
              onChange={(e) => setEdits((prev) => ({ ...prev, inboxTag: e.target.value }))}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="paperlessDoneTag">{t("paperless.pipeline.doneTag")}</Label>
            <Input
              id="paperlessDoneTag"
              value={current.doneTag}
              onChange={(e) => setEdits((prev) => ({ ...prev, doneTag: e.target.value }))}
              required
            />
          </div>
        </div>
        <p className="text-sm text-muted-foreground">{t("paperless.pipeline.tagsHint")}</p>
        <div className="flex flex-wrap gap-3">
          <Button type="submit" variant="outline" disabled={updateSettings.isPending || Object.keys(edits).length === 0}>
            {t("paperless.pipeline.saveTags")}
          </Button>
          <Button type="button" variant="outline" onClick={handleRun} disabled={!settings.enabled || runPipeline.isPending}>
            {runPipeline.isPending ? t("paperless.pipeline.running") : t("paperless.pipeline.runNow")}
          </Button>
        </div>
      </form>
    </div>
  )
}
