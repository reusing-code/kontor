import { useTranslation } from "react-i18next"
import { useCapabilities } from "@/hooks/use-capabilities"

export function LedgerEmailLlmNotice() {
  const { t } = useTranslation()
  const { llm, isLoading } = useCapabilities()
  if (isLoading || llm) {
    return null
  }
  return (
    <div className="rounded-lg border bg-muted/50 p-3 text-sm text-muted-foreground">
      {t("ledger.email.llmRequired")}
    </div>
  )
}
