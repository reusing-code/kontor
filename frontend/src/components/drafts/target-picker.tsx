import { useTranslation } from "react-i18next"
import type { DraftEntityType, DraftTargetType } from "@/types/drafts"
import { parentKind, type DraftOption } from "@/components/drafts/draft-input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"

interface TargetPickerProps {
  target: DraftTargetType
  parentId?: string
  enabledTypes: DraftEntityType[]
  parentOptions: Record<DraftEntityType, DraftOption[]>
  disabled?: boolean
  onChangeType: (type: DraftTargetType) => void
  onChangeParent: (parentId: string) => void
}

// Select values cannot be empty, so "no parent" of a ledger category uses this.
const noParentValue = "__none__"

const parentLabelKeys = {
  category: "drafts.category",
  vehicle: "drafts.vehicle",
  ledgerCategory: "drafts.parentCategory",
} as const

const parentPlaceholderKeys = {
  category: "drafts.pickCategory",
  vehicle: "drafts.pickVehicle",
  ledgerCategory: "drafts.noParent",
} as const

// TargetPicker selects the entity type (enabled modules, the current type even
// if its module is disabled, and "none") and the category or vehicle.
export function TargetPicker({
  target,
  parentId,
  enabledTypes,
  parentOptions,
  disabled,
  onChangeType,
  onChangeParent,
}: TargetPickerProps) {
  const { t } = useTranslation()
  const options = target === "none" ? [] : parentOptions[target]
  const kind = parentKind(target)
  const optional = kind === "ledgerCategory"
  const parentValid = options.some((o) => o.id === parentId)
  const typeChoices: DraftTargetType[] = [
    ...enabledTypes,
    ...(target !== "none" && !enabledTypes.includes(target) ? [target] : []),
    "none",
  ]

  return (
    <>
      <Select value={target} onValueChange={(v) => onChangeType(v as DraftTargetType)} disabled={disabled}>
        <SelectTrigger className="w-44" aria-label={t("drafts.type")}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {typeChoices.map((type) => (
            <SelectItem key={type} value={type}>
              {t(`drafts.types.${type}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {kind !== "none" && options.length > 0 && (
        <Select
          value={parentValid ? parentId : optional ? noParentValue : ""}
          onValueChange={(v) => onChangeParent(v === noParentValue ? "" : v)}
          disabled={disabled}
        >
          <SelectTrigger className="w-52" aria-label={t(parentLabelKeys[kind])}>
            <SelectValue placeholder={t(parentPlaceholderKeys[kind])} />
          </SelectTrigger>
          <SelectContent>
            {optional && <SelectItem value={noParentValue}>{t("drafts.noParent")}</SelectItem>}
            {options.map((o) => (
              <SelectItem key={o.id} value={o.id}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </>
  )
}
