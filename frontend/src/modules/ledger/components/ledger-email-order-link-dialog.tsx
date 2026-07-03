import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { formatAmountMinor, formatLedgerDate, tokenizeLedgerMatchWords } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerEmailOrder, LedgerTransaction } from "@/modules/ledger/types"
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

interface LedgerEmailOrderLinkDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  order: LedgerEmailOrder | null
  transactions: LedgerTransaction[]
  onConfirm: (transactionIds: string[]) => void
}

type Candidate = {
  transaction: LedgerTransaction
  score: number
}

export function LedgerEmailOrderLinkDialog({ open, onOpenChange, order, transactions, onConfirm }: LedgerEmailOrderLinkDialogProps) {
  const { t, i18n } = useTranslation()
  const [selectedIds, setSelectedIds] = useState<string[]>([])

  const candidates = useMemo(() => {
    if (!order) {
      return []
    }
    const orderTokens = tokenizeLedgerMatchWords(order.externalOrderId, order.emailSubject, ...(order.items ?? []).map((item) => item.name))
    const result: Candidate[] = []

    for (const transaction of transactions) {
      if ((transaction.emailOrderIds ?? []).length > 0) {
        continue
      }
      if (transaction.amountMinor >= 0) {
        continue
      }

      let score = 0
      const amountMinor = Math.abs(transaction.amountMinor)
      const delta = Math.abs(amountMinor - Math.abs(order.totalMinor))
      if (delta === 0) {
        score += 100
      } else if (delta <= 200) {
        score += 50
      } else if (delta <= 1000) {
        score += 20
      }

      const orderDate = new Date(order.orderDate)
      const bookingDate = new Date(transaction.bookingDate)
      const dayDelta = Math.abs(Math.round((bookingDate.getTime() - orderDate.getTime()) / (1000 * 60 * 60 * 24)))
      if (dayDelta <= 1) {
        score += 30
      } else if (dayDelta <= 3) {
        score += 15
      } else if (dayDelta <= 7) {
        score += 5
      }

      const haystack = `${transaction.counterpartyName} ${transaction.purpose}`.toLowerCase()
      for (const token of orderTokens) {
        if (token.length >= 4 && haystack.includes(token)) {
          score += 8
        }
      }

      if (score > 0) {
        result.push({ transaction, score })
      }
    }

    return result.sort((left, right) => right.score - left.score).slice(0, 12)
  }, [order, transactions])

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

            <div className="space-y-2 max-h-[24rem] overflow-auto">
              {candidates.length === 0 ? <p className="text-sm text-muted-foreground">{t("ledger.email.noCandidates")}</p> : null}
              {candidates.map(({ transaction, score }) => {
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
                        <span className="text-sm text-muted-foreground">{formatLedgerDate(transaction.bookingDate)}</span>
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
