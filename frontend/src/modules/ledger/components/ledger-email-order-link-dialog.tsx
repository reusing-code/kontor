import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useLedgerEmailOrderCandidates } from "@/modules/ledger/hooks/use-ledger"
import { formatAmountMinor, formatLedgerDate, tokenizeLedgerMatchWords } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerEmailOrder, LedgerEmailOrderCandidate } from "@/modules/ledger/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"

const extendedAfterDays = 365
const maxShownCandidates = 30

interface LedgerEmailOrderLinkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: LedgerEmailOrder | null
  onConfirm: (transactionIds: string[]) => void
}

type ScoredCandidate = LedgerEmailOrderCandidate & { score: number }

function scoreCandidate(order: LedgerEmailOrder, orderTokens: string[], candidate: LedgerEmailOrderCandidate, windowDays: number): number {
  const { transaction, dateDeltaDays } = candidate
  let score = 0
  const delta = Math.abs(Math.abs(transaction.amountMinor) - Math.abs(order.totalMinor))
  if (delta === 0) {
    score += 100
  } else if (delta <= 200) {
    score += 50
  } else if (delta <= 1000) {
    score += 20
  }

  if (dateDeltaDays !== undefined) {
    score += Math.round(30 * Math.max(0, 1 - Math.abs(dateDeltaDays) / Math.max(windowDays, 1)))
  }

  const haystack = `${transaction.counterpartyName} ${transaction.purpose}`.toLowerCase()
  for (const token of orderTokens) {
    if (token.length >= 4 && haystack.includes(token)) {
      score += 8
    }
  }
  return score
}

function formatDayDelta(days: number): string {
  return days > 0 ? `+${days}` : `${days}`
}

// Mount with key={order.id} so selection and window reset per order.
export function LedgerEmailOrderLinkDialog({ open, onOpenChange, order, onConfirm }: LedgerEmailOrderLinkDialogProps) {
  const { t, i18n } = useTranslation()
  const [selectedIds, setSelectedIds] = useState<string[]>([])
  const [afterDays, setAfterDays] = useState<number | undefined>(undefined)
  const { data: result, isLoading } = useLedgerEmailOrderCandidates(open && order ? order.id : "", afterDays)

  const candidates = useMemo(() => {
    if (!order || !result) {
      return []
    }
    const orderTokens = tokenizeLedgerMatchWords(order.externalOrderId, order.emailSubject, ...(order.items ?? []).map((item) => item.name))
    return result.items
      .map((candidate): ScoredCandidate => ({ ...candidate, score: scoreCandidate(order, orderTokens, candidate, result.afterDays) }))
      .sort((left, right) => right.score - left.score)
      .slice(0, maxShownCandidates)
  }, [order, result])

  function toggle(id: string, checked: boolean) {
    setSelectedIds((current) => checked ? [...current, id] : current.filter((value) => value !== id))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t("ledger.email.reviewMatch")}</DialogTitle>
          <DialogDescription>{t("ledger.email.reviewMatchDescription")}</DialogDescription>
        </DialogHeader>

        {order ? (
          <div className="space-y-4">
            <div className="rounded-md border p-4 space-y-2">
              <div className="font-medium">{order.externalOrderId || order.emailSubject || order.importerId}</div>
              <div className="text-sm text-muted-foreground">{formatLedgerDate(order.orderDate)} • {formatAmountMinor(order.totalMinor, order.currency, i18n.language)}</div>
              <div className="space-y-1 text-sm text-muted-foreground">
                {(order.items ?? []).map((item, index) => (
                  <div key={`${order.id}-${index}`}>{item.quantity}x {item.name}</div>
                ))}
              </div>
            </div>

            {result && order.orderDate ? (
              <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
                <span>{t("ledger.email.candidateWindow", { before: result.beforeDays, after: result.afterDays })}</span>
                {afterDays === undefined && result.afterDays < extendedAfterDays ? (
                  <Button type="button" variant="outline" size="sm" onClick={() => setAfterDays(extendedAfterDays)}>{t("ledger.email.showLaterCharges")}</Button>
                ) : null}
              </div>
            ) : null}

            <div className="space-y-2 max-h-[24rem] overflow-auto">
              {!isLoading && candidates.length === 0 ? <p className="text-sm text-muted-foreground">{t("ledger.email.noCandidates")}</p> : null}
              {candidates.map(({ transaction, accountName, dateDeltaDays, score }) => {
                const checked = selectedIds.includes(transaction.id)
                return (
                  <button key={transaction.id} type="button" className="flex w-full items-start gap-3 rounded-md border p-3 text-left" onClick={() => toggle(transaction.id, !checked)}>
                    <div className="pt-1">
                      <Badge variant={checked ? "default" : "outline"}>{checked ? t("common.selected") : t("common.select")}</Badge>
                    </div>
                    <div className="min-w-0 flex-1 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}</span>
                        <Badge variant="outline">{t("ledger.email.matchScore", { score })}</Badge>
                        <span className="text-sm text-muted-foreground">
                          {formatLedgerDate(transaction.bookingDate)}
                          {dateDeltaDays !== undefined ? ` (${t("ledger.email.dayDelta", { delta: formatDayDelta(dateDeltaDays) })})` : null}
                        </span>
                        {accountName ? <Badge variant="secondary">{accountName}</Badge> : null}
                      </div>
                      <div className="text-sm">{transaction.counterpartyName || t("ledger.noCounterparty")}</div>
                      <div className="text-sm text-muted-foreground break-words">{transaction.purpose || "-"}</div>
                    </div>
                  </button>
                )
              })}
            </div>
          </div>
        ) : null}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
          <Button type="button" onClick={() => onConfirm(selectedIds)} disabled={selectedIds.length === 0}>{t("ledger.email.linkSelected")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
