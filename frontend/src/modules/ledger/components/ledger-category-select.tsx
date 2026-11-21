import { useMemo, type ComponentProps } from "react"
import type { LedgerCategory } from "@/modules/ledger/types"
import { categoryPathSeparator, flattenCategoryTree } from "@/modules/ledger/lib/category-tree"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

const noneValue = "__none__"

type TriggerProps = Omit<ComponentProps<typeof SelectTrigger>, "children" | "onChange" | "value" | "defaultValue">

interface LedgerCategorySelectProps extends TriggerProps {
  categories: LedgerCategory[]
  // "" means no category
  value: string
  onValueChange: (value: string) => void
  // offers "no category" with this label when set
  noneLabel?: string
  placeholder?: string
  // categories that cannot be picked (e.g. a category and its subtree as its own parent)
  exclude?: ReadonlySet<string>
}

// LedgerCategorySelect offers the ledger categories as an indented tree and
// shows the full path of the chosen one. Extra props go to the trigger, so it
// works inside FormControl.
export function LedgerCategorySelect({
  categories,
  value,
  onValueChange,
  noneLabel,
  placeholder,
  exclude,
  className,
  ...triggerProps
}: LedgerCategorySelectProps) {
  const entries = useMemo(() => flattenCategoryTree(categories), [categories])
  const selected = entries.find((entry) => entry.category.id === value)
  const selectValue = selected ? value : noneLabel !== undefined ? noneValue : ""

  return (
    <Select value={selectValue} onValueChange={(next) => onValueChange(next === noneValue ? "" : next)}>
      <SelectTrigger className={className ?? "w-full"} {...triggerProps}>
        <SelectValue placeholder={placeholder}>
          {selected ? selected.path.join(categoryPathSeparator) : noneLabel}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {noneLabel !== undefined && <SelectItem value={noneValue}>{noneLabel}</SelectItem>}
        {entries
          .filter((entry) => !exclude?.has(entry.category.id))
          .map((entry) => (
            <SelectItem key={entry.category.id} value={entry.category.id} style={{ paddingLeft: `${0.5 + entry.depth * 1.25}rem` }}>
              {entry.category.name}
            </SelectItem>
          ))}
      </SelectContent>
    </Select>
  )
}
