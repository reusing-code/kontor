import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useLedgerTransaction, useLedgerTransferCandidates, useLinkLedgerTransfer, useUnlinkLedgerTransfer } from "@/modules/ledger/hooks/use-ledger"
import { transactionPath } from "@/modules/ledger/lib/module-links"
import { formatAmountMinor, formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerTransaction } from "@/modules/ledger/types"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"

interface LedgerTransferManagerProps {
  transaction: LedgerTransaction
  compact?: boolean
}

export function LedgerTransferManager({ transaction, compact = false }: LedgerTransferManagerProps) {
  const { t, i18n } = useTranslation()
  const { data: transferCandidates } = useLedgerTransferCandidates(transaction.id)
  const pairedTransactionId = transaction.transferPairTransactionId
  const { data: pairedTransaction } = useLedgerTransaction(pairedTransactionId ?? "")
  const linkTransfer = useLinkLedgerTransfer()
  const unlinkTransfer = useUnlinkLedgerTransfer()
  const visibleCandidates = transferCandidates?.items.filter((candidate) => candidate.transaction.id !== pairedTransactionId) ?? []

  return (
    <div className="space-y-4">
      {pairedTransaction ? (
        <div className="rounded-md border p-3 text-sm">
          <div className="font-medium">{t("ledger.transferLinked")}</div>
          <Link to={transactionPath(pairedTransaction.id)} className="mt-1 block text-primary hover:underline">
            {formatLedgerDate(pairedTransaction.bookingDate)} • {formatAmountMinor(pairedTransaction.amountMinor, pairedTransaction.currency, i18n.language)}
          </Link>
          <div className="text-muted-foreground">{pairedTransaction.counterpartyName || pairedTransaction.purpose || "-"}</div>
          <p className="mt-2 text-xs text-muted-foreground">{t("ledger.transferCategoryProtection")}</p>
          <div className="mt-3">
            <AlertDialog>
              <AlertDialogTrigger asChild>
                <Button type="button" variant="outline">{t("ledger.unlinkTransfer")}</Button>
              </AlertDialogTrigger>
              <AlertDialogContent>
                <AlertDialogHeader>
                  <AlertDialogTitle>{t("ledger.unlinkTransfer")}</AlertDialogTitle>
                  <AlertDialogDescription>{t("ledger.unlinkTransferConfirm")}</AlertDialogDescription>
                </AlertDialogHeader>
                <AlertDialogFooter>
                  <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
                  <AlertDialogAction
                    onClick={() => unlinkTransfer.mutate(transaction.id, {
                      onSuccess: () => toast.success(t("ledger.transferUnlinked")),
                      onError: (error) => toast.error(error.message),
                    })}
                  >
                    {t("ledger.unlinkTransfer")}
                  </AlertDialogAction>
                </AlertDialogFooter>
              </AlertDialogContent>
            </AlertDialog>
          </div>
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">{t("ledger.noTransferLinked")}</p>
      )}

      <div className="space-y-2">
        <div className="text-sm font-medium">{t("ledger.transferCandidates")}</div>
        {visibleCandidates.length ? (
          visibleCandidates.map((candidate) => (
            <div key={candidate.transaction.id} className="flex items-center gap-3 rounded-md border p-3 text-sm">
              <div className="min-w-0 flex-1">
                <div className="font-medium">{candidate.accountName}</div>
                <div>{formatLedgerDate(candidate.transaction.bookingDate)} • {formatAmountMinor(candidate.transaction.amountMinor, candidate.transaction.currency, i18n.language)}</div>
                <div className="text-xs text-muted-foreground">
                  {candidate.dateDeltaDays === 0 ? t("ledger.sameDay") : t("ledger.daysApart", { count: candidate.dateDeltaDays })}
                  {candidate.ibanMatch ? ` • ${t("ledger.ibanMatched")}` : ""}
                </div>
              </div>
              <Button
                type="button"
                variant="outline"
                size={compact ? "sm" : "default"}
                onClick={() => linkTransfer.mutate({ id: transaction.id, data: { pairedTransactionId: candidate.transaction.id } }, {
                  onSuccess: () => toast.success(t("ledger.transferLinkedSuccess")),
                  onError: (error) => toast.error(error.message),
                })}
                disabled={linkTransfer.isPending || Boolean(pairedTransactionId)}
              >
                {t("ledger.linkTransfer")}
              </Button>
            </div>
          ))
        ) : (
          <p className="text-sm text-muted-foreground">{t(pairedTransactionId ? "ledger.unlinkToSeeTransferCandidates" : "ledger.noTransferCandidates")}</p>
        )}
      </div>
    </div>
  )
}
