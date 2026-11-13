import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useBankAspsps } from "@/modules/ledger/hooks/use-bank-connections"
import { bankErrorMessage } from "@/modules/ledger/hooks/use-bank-authorization"
import type { BankInfo } from "@/modules/ledger/types"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { cn } from "@/lib/utils"

const COUNTRIES = ["DE", "AT", "BE", "CH", "CZ", "DK", "ES", "FI", "FR", "GB", "IE", "IT", "LU", "NL", "NO", "PL", "PT", "SE"]

interface BankConnectDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  pending: boolean
  onConfirm: (bank: BankInfo) => void
}

export function BankConnectDialog({ open, onOpenChange, pending, onConfirm }: BankConnectDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <BankConnectDialogContent onOpenChange={onOpenChange} pending={pending} onConfirm={onConfirm} /> : null}
    </Dialog>
  )
}

function BankConnectDialogContent({ onOpenChange, pending, onConfirm }: Omit<BankConnectDialogProps, "open">) {
  const { t } = useTranslation()
  const [country, setCountry] = useState("DE")
  const [search, setSearch] = useState("")
  const [selected, setSelected] = useState<BankInfo | null>(null)
  const { data: banks = [], isLoading, error } = useBankAspsps(country, true)

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase()
    return banks
      .filter((bank) => bank.name.toLowerCase().includes(needle))
      .sort((a, b) => Number(b.verified) - Number(a.verified) || a.name.localeCompare(b.name))
  }, [banks, search])

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t("ledger.bankImport.connect.title")}</DialogTitle>
        <DialogDescription>{t("ledger.bankImport.connect.description")}</DialogDescription>
      </DialogHeader>
      <div className="space-y-3">
        <div className="flex gap-2">
          <Select
            value={country}
            onValueChange={(value) => {
              setCountry(value)
              setSelected(null)
            }}
          >
            <SelectTrigger className="w-24" aria-label={t("ledger.bankImport.connect.country")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {COUNTRIES.map((code) => (
                <SelectItem key={code} value={code}>
                  {code}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t("ledger.bankImport.connect.search")}
            aria-label={t("ledger.bankImport.connect.search")}
          />
        </div>

        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {bankErrorMessage(t, error)}
          </p>
        ) : isLoading ? (
          <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
        ) : visible.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("ledger.bankImport.connect.none")}</p>
        ) : (
          <ul className="max-h-72 space-y-1 overflow-y-auto">
            {visible.map((bank) => (
              <li key={`${bank.country}-${bank.name}`}>
                <button
                  type="button"
                  onClick={() => setSelected(bank)}
                  className={cn(
                    "flex w-full items-center justify-between gap-2 rounded-md border px-3 py-2 text-left text-sm hover:bg-accent",
                    selected?.name === bank.name && "border-primary bg-accent",
                  )}
                >
                  <span>{bank.name}</span>
                  {bank.verified && <Badge variant="secondary">{t("ledger.bankImport.connect.verified")}</Badge>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          {t("common.cancel")}
        </Button>
        <Button type="button" disabled={!selected || pending} onClick={() => selected && onConfirm(selected)}>
          {t("ledger.bankImport.connect.confirm")}
        </Button>
      </DialogFooter>
    </DialogContent>
  )
}
