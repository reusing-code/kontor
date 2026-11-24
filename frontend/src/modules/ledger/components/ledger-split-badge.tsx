import { useTranslation } from "react-i18next"
import { formatMinor } from "@/lib/format"
import type { LedgerCategory, LedgerTransaction } from "@/modules/ledger/types"
import { Badge } from "@/components/ui/badge"

// LedgerSplitBadge marks a split transaction; its title lists the parts.
export function LedgerSplitBadge({ transaction, categories }: { transaction: LedgerTransaction; categories: LedgerCategory[] }) {
  const { t } = useTranslation()
  const parts = transaction.splits ?? []
  if (parts.length === 0) return null
  const name = (id?: string) => categories.find((category) => category.id === id)?.name ?? t("ledger.noCategory")
  const title = parts
    .map((part) => `${formatMinor(part.amountMinor, transaction.currency)} ${name(part.categoryId)}${part.note ? ` (${part.note})` : ""}`)
    .join("\n")
  return (
    <Badge variant="outline" title={title}>
      {t("ledger.split.badge", { count: parts.length })}
    </Badge>
  )
}
