import { useState, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import type { LedgerManualBalanceInput } from "@/modules/ledger/types"
import { parseBalanceInput, todayISO } from "@/modules/ledger/lib/balance"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"

interface LedgerBalanceDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  pending?: boolean
  onSubmit: (data: LedgerManualBalanceInput) => void
}

export function LedgerBalanceDialog({ open, onOpenChange, pending, onSubmit }: LedgerBalanceDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <BalanceForm onOpenChange={onOpenChange} pending={pending} onSubmit={onSubmit} /> : null}
    </Dialog>
  )
}

function BalanceForm({ onOpenChange, pending, onSubmit }: Omit<LedgerBalanceDialogProps, "open">) {
  const { t } = useTranslation()
  const today = todayISO()
  const [amount, setAmount] = useState("")
  const [asOf, setAsOf] = useState(today)
  const [submitted, setSubmitted] = useState(false)
  const amountMinor = parseBalanceInput(amount)
  const dateValid = /^\d{4}-\d{2}-\d{2}$/.test(asOf) && asOf <= today

  function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setSubmitted(true)
    if (amountMinor === undefined || !dateValid) return
    onSubmit({ amountMinor, asOf })
  }

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t("ledger.balance.dialogTitle")}</DialogTitle>
        <DialogDescription>{t("ledger.balance.dialogDescription")}</DialogDescription>
      </DialogHeader>
      <form className="space-y-4" onSubmit={handleSubmit} noValidate>
        <div className="space-y-2">
          <Label htmlFor="ledger-balance-amount">{t("ledger.balance.amount")}</Label>
          <Input
            id="ledger-balance-amount"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            aria-invalid={submitted && amountMinor === undefined}
            autoFocus
          />
          {submitted && amountMinor === undefined && <p className="text-sm text-destructive">{t("ledger.balance.amountInvalid")}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="ledger-balance-date">{t("ledger.balance.date")}</Label>
          <Input id="ledger-balance-date" type="date" max={today} value={asOf} onChange={(e) => setAsOf(e.target.value)} aria-invalid={submitted && !dateValid} />
          {submitted && !dateValid && <p className="text-sm text-destructive">{t("ledger.balance.dateInvalid")}</p>}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={pending}>
            {t("common.save")}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  )
}
