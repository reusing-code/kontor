import { useState } from "react"
import { formatMinor } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { PlusIcon, Trash2Icon } from "lucide-react"
import type { LedgerCategory, LedgerTransaction } from "@/modules/ledger/types"
import { useSetLedgerTransactionSplits } from "@/modules/ledger/hooks/use-ledger"
import {
  maxSplitParts,
  rowsFromSplits,
  splitProblem,
  splitRemainder,
  splitsFromRows,
  type SplitRow,
} from "@/modules/ledger/lib/splits"
import { LedgerCategorySelect } from "@/modules/ledger/components/ledger-category-select"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

interface LedgerSplitEditorProps {
  transaction: LedgerTransaction
  categories: LedgerCategory[]
}

const emptyRow: SplitRow = { amount: "", categoryId: "", note: "" }

// LedgerSplitEditor books parts of a transaction to other categories (e.g.
// the cash withdrawal inside a supermarket payment). The rest stays with the
// transaction's own category. Saving stores the split right away. Linked
// internal transfers cannot be split.
export function LedgerSplitEditor({ transaction, categories }: LedgerSplitEditorProps) {
  const { t, i18n } = useTranslation()
  const setSplits = useSetLedgerTransactionSplits()
  // what the server has, so the editor knows about unsaved changes without
  // waiting for the transaction to be refetched
  const [saved, setSaved] = useState<SplitRow[]>(() => rowsFromSplits(transaction.splits, i18n.language))
  const [rows, setRows] = useState<SplitRow[]>(saved)

  if (transaction.transferPairTransactionId) {
    return <p className="text-sm text-muted-foreground">{t("ledger.split.transferLinked")}</p>
  }

  const problem = splitProblem(rows, transaction.amountMinor)
  const changed = JSON.stringify(rows) !== JSON.stringify(saved)
  const remainder = splitRemainder(rows, transaction.amountMinor)
  const mainCategory = categories.find((category) => category.id === transaction.categoryId)?.name ?? t("ledger.noCategory")
  const money = (minor: number) => formatMinor(minor, transaction.currency)

  function update(index: number, patch: Partial<SplitRow>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  function save() {
    const splits = splitsFromRows(rows, transaction.amountMinor)
    setSplits.mutate({ id: transaction.id, splits }, {
      onSuccess: (updated) => {
        const stored = rowsFromSplits(updated.splits, i18n.language)
        setSaved(stored)
        setRows(stored)
        toast.success(t(splits.length > 0 ? "ledger.split.saved" : "ledger.split.removed"))
      },
    })
  }

  return (
    <div className="space-y-3">
      {rows.length === 0 && <p className="text-sm text-muted-foreground">{t("ledger.split.description")}</p>}
      {rows.map((row, index) => (
        <div key={index} className="grid gap-2 sm:grid-cols-[7rem_minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-center">
          <Input
            value={row.amount}
            inputMode="decimal"
            placeholder="0,00"
            onChange={(event) => update(index, { amount: event.target.value })}
            aria-label={t("ledger.split.amount", { index: index + 1 })}
            aria-invalid={row.amount !== "" && problem === "amount" ? true : undefined}
          />
          <LedgerCategorySelect
            categories={categories}
            value={row.categoryId}
            onValueChange={(categoryId) => update(index, { categoryId })}
            placeholder={t("ledger.selectCategory")}
            aria-label={t("ledger.split.category", { index: index + 1 })}
          />
          <Input
            value={row.note}
            onChange={(event) => update(index, { note: event.target.value })}
            placeholder={t("ledger.split.notePlaceholder")}
            aria-label={t("ledger.split.note", { index: index + 1 })}
          />
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => setRows((current) => current.filter((_, i) => i !== index))}
            aria-label={t("ledger.split.removePart", { index: index + 1 })}
          >
            <Trash2Icon />
          </Button>
        </div>
      ))}

      {rows.length > 0 && (
        <p className={problem === "total" ? "text-sm text-destructive" : "text-sm text-muted-foreground"}>
          {t("ledger.split.remainder", { amount: money(remainder), category: mainCategory })}
        </p>
      )}
      {problem && <p className="text-sm text-destructive">{t(`ledger.split.problems.${problem}`)}</p>}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setRows((current) => [...current, emptyRow])}
          disabled={rows.length >= maxSplitParts || transaction.amountMinor === 0}
        >
          <PlusIcon />
          {t("ledger.split.addPart")}
        </Button>
        {changed && (
          <Button type="button" size="sm" onClick={save} disabled={problem !== null || setSplits.isPending}>
            {rows.length === 0 ? t("ledger.split.remove") : t("ledger.split.save")}
          </Button>
        )}
      </div>
    </div>
  )
}
