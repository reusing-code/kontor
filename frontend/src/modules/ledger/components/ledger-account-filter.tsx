import { useTranslation } from "react-i18next"
import { ChevronDown } from "lucide-react"
import type { LedgerAccount } from "@/modules/ledger/types"
import { toggleAccount } from "@/modules/ledger/lib/transaction-filter"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"

interface LedgerAccountFilterProps {
  accounts: LedgerAccount[]
  // empty = all accounts
  selected: string[]
  onChange: (selected: string[]) => void
}

export function LedgerAccountFilter({ accounts, selected, onChange }: LedgerAccountFilterProps) {
  const { t } = useTranslation()
  const label = selected.length === 0
    ? t("ledger.allTransactions.allAccounts")
    : selected.length === 1
      ? accounts.find((account) => account.id === selected[0])?.name ?? t("ledger.allTransactions.accountCount", { count: 1 })
      : t("ledger.allTransactions.accountCount", { count: selected.length })

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="outline" aria-label={t("ledger.allTransactions.filterLabel", { selection: label })}>
          {label}
          <ChevronDown className="ml-2 h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-96 overflow-y-auto">
        <DropdownMenuItem disabled={selected.length === 0} onSelect={() => onChange([])}>
          {t("ledger.allTransactions.showAll")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {accounts.map((account) => (
          <DropdownMenuCheckboxItem
            key={account.id}
            checked={selected.includes(account.id)}
            onSelect={(event) => event.preventDefault()}
            onCheckedChange={() => onChange(toggleAccount(selected, account.id))}
          >
            {account.name}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
