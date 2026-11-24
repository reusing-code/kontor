import { useTranslation } from "react-i18next"
import type { LedgerAccount } from "@/modules/ledger/types"
import { useLedgerAccountDeleteSummary } from "@/modules/ledger/hooks/use-ledger-accounts"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

interface LedgerAccountDeleteDialogProps {
  account: LedgerAccount
  open: boolean
  onOpenChange: (open: boolean) => void
  pending?: boolean
  onConfirm: () => void
}

// LedgerAccountDeleteDialog lists what deleting the account removes before
// it asks; the counts are loaded when the dialog opens.
export function LedgerAccountDeleteDialog({ account, open, onOpenChange, pending, onConfirm }: LedgerAccountDeleteDialogProps) {
  const { t } = useTranslation()
  const { data: summary, isLoading, error } = useLedgerAccountDeleteSummary(account.id, open)

  const lines = summary
    ? [
        t("ledger.accountDelete.transactions", { count: summary.transactions }),
        t("ledger.accountDelete.imports", { count: summary.imports }),
        ...(summary.references > 0 ? [t("ledger.accountDelete.references", { count: summary.references })] : []),
        ...(summary.transfers > 0 ? [t("ledger.accountDelete.transfers", { count: summary.transfers })] : []),
        ...(summary.emailOrders > 0 ? [t("ledger.accountDelete.emailOrders", { count: summary.emailOrders })] : []),
        ...summary.bankAccounts.map((b) => t("ledger.accountDelete.bankAccount", { bank: b.bank, name: b.name || b.iban })),
      ]
    : []

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("ledger.accountDelete.title", { name: account.name })}</AlertDialogTitle>
          <AlertDialogDescription>{t("ledger.accountDelete.description")}</AlertDialogDescription>
        </AlertDialogHeader>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
        ) : error ? (
          <p role="alert" className="text-sm text-destructive">{error.message}</p>
        ) : (
          <ul className="list-disc space-y-1 pl-5 text-sm">
            {lines.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        )}
        <p className="text-sm text-muted-foreground">{t("ledger.accountDelete.afterwards")}</p>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="bg-destructive text-white hover:bg-destructive/90"
            disabled={!summary || pending}
            onClick={(event) => {
              event.preventDefault()
              onConfirm()
            }}
          >
            {pending ? t("ledger.accountDelete.deleting") : t("ledger.accountDelete.confirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
