import { useTranslation } from "react-i18next"
import type { DraftEntityType, DraftTargetType } from "@/types/drafts"
import type { DraftOption } from "@/components/drafts/draft-input"
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
      {target !== "none" && options.length > 0 && (
        <Select value={parentValid ? parentId : ""} onValueChange={onChangeParent} disabled={disabled}>
          <SelectTrigger className="w-52" aria-label={t(target === "cost" ? "drafts.vehicle" : "drafts.category")}>
            <SelectValue placeholder={t(target === "cost" ? "drafts.pickVehicle" : "drafts.pickCategory")} />
          </SelectTrigger>
          <SelectContent>
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
