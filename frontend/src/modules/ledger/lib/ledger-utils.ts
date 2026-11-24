import i18n from "@/i18n"
import type { LedgerCategorizationSource, LedgerEmailOrder, LedgerReviewStatus } from "@/modules/ledger/types"

export function emailOrderTitle(order: LedgerEmailOrder): string {
  return [order.merchant, order.externalOrderId].filter(Boolean).join(" · ") || order.emailSubject || order.importerId
}

const sourceTypeKeys: Record<string, string> = {
  "dkb.csv": "ledger.sourceTypes.dkbCsv",
  "comdirect.csv": "ledger.sourceTypes.comdirectCsv",
  enablebanking: "ledger.sourceTypes.enablebanking",
}

export function formatSourceType(sourceType: string): string {
  const key = sourceTypeKeys[sourceType]
  return key ? i18n.t(key) : sourceType
}

export function formatLedgerReviewStatus(reviewStatus: LedgerReviewStatus): string {
  return i18n.t(`ledger.reviewStatus.${reviewStatus}`, { defaultValue: reviewStatus })
}

export function formatLedgerCategorizationSource(source: LedgerCategorizationSource): string {
  return i18n.t(`ledger.categorizationSource.${source}`, { defaultValue: source })
}

export function formatLedgerSpecialCategory(specialCategory?: string): string {
  if (specialCategory === "internalTransfer") return i18n.t("ledger.internalTransfer")
  return specialCategory ?? ""
}

export function tokenizeLedgerMatchWords(...values: Array<string | undefined>): string[] {
  const seen = new Set<string>()
  const tokens: string[] = []

	for (const value of values) {
		if (!value) {
			continue
		}
		for (const part of value.split(/\s+/u)) {
			const token = part.trim().toLowerCase()
			if (!token || seen.has(token)) {
				continue
      }
      seen.add(token)
      tokens.push(token)
    }
  }

  return tokens
}
