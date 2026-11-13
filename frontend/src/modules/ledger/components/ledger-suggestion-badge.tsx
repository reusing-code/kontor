import { useTranslation } from "react-i18next"
import { SparklesIcon } from "lucide-react"
import type { LedgerSuggestionConfidence } from "@/modules/ledger/types"
import { Badge } from "@/components/ui/badge"

const variantByConfidence = {
  high: "default",
  medium: "secondary",
  low: "outline",
} as const

export function LedgerSuggestionBadge({ confidence }: { confidence: LedgerSuggestionConfidence }) {
  const { t } = useTranslation()
  return (
    <Badge variant={variantByConfidence[confidence]} title={t("ledger.suggestions.badgeTitle")}>
      <SparklesIcon />
      {t(`ledger.suggestions.confidence.${confidence}`)}
    </Badge>
  )
}
