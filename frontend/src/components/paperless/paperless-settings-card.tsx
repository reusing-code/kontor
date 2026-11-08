import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import {
  usePaperlessConfig,
  useUpdatePaperlessConfig,
  useDeletePaperlessConfig,
  useTestPaperlessConfig,
} from "@/hooks/use-paperless"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { useCapabilities } from "@/hooks/use-capabilities"
import { PaperlessPipelineSection } from "@/components/paperless/paperless-pipeline-section"

export function PaperlessSettingsCard() {
  const { t } = useTranslation()
  const { data: config } = usePaperlessConfig()
  const updateConfig = useUpdatePaperlessConfig()
  const deleteConfig = useDeletePaperlessConfig()
  const testConfig = useTestPaperlessConfig()
  const { llm } = useCapabilities()

  const [baseUrl, setBaseUrl] = useState<string | null>(null)
  const [token, setToken] = useState("")

  const configured = config?.configured ?? false
  const displayBaseUrl = baseUrl ?? config?.baseUrl ?? ""

  function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!configured && token.trim() === "") {
      toast.error(t("paperless.tokenRequired"))
      return
    }
    updateConfig.mutate(
      { baseUrl: displayBaseUrl, token: token.trim() === "" ? undefined : token.trim() },
      {
        onSuccess: () => {
          toast.success(t("paperless.saved"))
          setToken("")
        },
        onError: (err) => toast.error(err.message || t("settings.saveFailed")),
      },
    )
  }

  function handleTest() {
    testConfig.mutate(undefined, {
      onSuccess: () => toast.success(t("paperless.testSuccess")),
      onError: (err) => toast.error(t("paperless.testFailed", { message: err.message })),
    })
  }

  function handleRemove() {
    deleteConfig.mutate(undefined, {
      onSuccess: () => {
        toast.success(t("paperless.removed"))
        setBaseUrl(null)
        setToken("")
      },
      onError: (err) => toast.error(err.message || t("settings.saveFailed")),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("paperless.title")}</CardTitle>
        <CardDescription>{t("paperless.settingsDescription")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <form onSubmit={handleSave} className="space-y-4 max-w-xl">
          <div className="space-y-2">
            <Label htmlFor="paperlessBaseUrl">{t("paperless.baseUrl")}</Label>
            <Input
              id="paperlessBaseUrl"
              type="text"
              placeholder="https://paperless.example.com"
              value={displayBaseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="paperlessToken">{t("paperless.token")}</Label>
            <Input
              id="paperlessToken"
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={configured ? t("paperless.tokenUnchangedHint") : undefined}
              autoComplete="off"
            />
          </div>
          <div className="flex flex-wrap gap-3">
            <Button type="submit" disabled={updateConfig.isPending}>
              {t("common.save")}
            </Button>
            {configured && (
              <>
                <Button type="button" variant="outline" onClick={handleTest} disabled={testConfig.isPending}>
                  {testConfig.isPending ? t("paperless.testing") : t("paperless.testConnection")}
                </Button>
                <Button type="button" variant="destructive" onClick={handleRemove} disabled={deleteConfig.isPending}>
                  {t("paperless.remove")}
                </Button>
              </>
            )}
          </div>
          {configured && <p className="text-sm text-muted-foreground">{t("paperless.removeHint")}</p>}
        </form>
        {configured && llm && <PaperlessPipelineSection />}
      </CardContent>
    </Card>
  )
}
