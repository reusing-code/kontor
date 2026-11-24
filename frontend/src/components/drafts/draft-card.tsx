import { useTranslation } from "react-i18next"
import { formatDateTime, formatMoney } from "@/lib/format"
import { ExternalLink } from "lucide-react"
import type { Draft, DraftEntityType, DraftTargetType } from "@/types/drafts"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { PaperlessThumbnail } from "@/components/paperless/paperless-thumbnail"
import { targetProblem, type DraftOption, type TargetProblem } from "@/components/drafts/draft-input"
import { TargetPicker } from "@/components/drafts/target-picker"

const hintKeys: Record<TargetProblem, string> = {
  pickType: "drafts.hintPickType",
  moduleDisabled: "drafts.hintModuleDisabled",
  noCategories: "drafts.hintNoCategories",
  noVehicles: "drafts.hintNoVehicles",
  pickCategory: "drafts.hintPickCategory",
  pickVehicle: "drafts.hintPickVehicle",
}

interface DraftCardProps {
  draft: Draft
  enabledTypes: DraftEntityType[]
  parentOptions: Record<DraftEntityType, DraftOption[]>
  busy: boolean
  // Bulk selection; only drafts that could be accepted as they are can be selected.
  selected: boolean
  onSelectedChange: (selected: boolean) => void
  // Why the last bulk accept left this draft pending.
  error?: string
  onChangeType: (type: DraftTargetType) => void
  onChangeParent: (parentId: string) => void
  onAccept: () => void
  onReject: () => void
}

type Field = [labelKey: string, value: unknown, kind?: "money" | "costType" | "billingInterval"]

function summaryFields(draft: Draft): Field[] {
  const input = draft.input
  switch (draft.targetType) {
    case "purchase":
      return [
        ["purchaseFields.itemName", input.itemName],
        ["purchaseFields.dealer", input.dealer],
        ["purchaseFields.price", input.price, "money"],
        ["purchaseFields.purchaseDate", input.purchaseDate],
      ]
    case "contract":
      return [
        ["fields.name", input.name],
        ["fields.company", input.company],
        ["fields.price", input.price, "money"],
        ["fields.billingInterval", input.billingInterval, "billingInterval"],
        ["fields.startDate", input.startDate],
        ["fields.endDate", input.endDate],
      ]
    case "cost":
      return [
        ["costEntryFields.type", input.type, "costType"],
        ["costEntryFields.vendor", input.vendor],
        ["costEntryFields.amount", input.amount, "money"],
        ["costEntryFields.date", input.date],
      ]
    case "vehicle":
      return [
        ["vehicleFields.name", input.name],
        ["vehicleFields.make", input.make],
        ["vehicleFields.model", input.model],
        ["vehicleFields.licensePlate", input.licensePlate],
      ]
    case "contract_category":
    case "purchase_category":
      return [["category.name", input.name]]
    case "ledger_category":
      return [
        ["category.name", input.name],
        ["ledger.matchWords", Array.isArray(input.matchWords) ? input.matchWords.join(", ") : undefined],
      ]
    case "ledger_account":
      return [
        ["ledger.accountName", input.name],
        ["ledger.bank", input.bank],
        ["ledger.iban", input.iban],
        ["drafts.currency", input.currency],
      ]
    default:
      return []
  }
}

export function DraftCard({
  draft,
  enabledTypes,
  parentOptions,
  busy,
  selected,
  onSelectedChange,
  error,
  onChangeType,
  onChangeParent,
  onAccept,
  onReject,
}: DraftCardProps) {
  const { t } = useTranslation()
  const target = draft.targetType

  const documentId = draft.source.kind === "paperless" ? Number(draft.source.ref) : NaN

  function display([, value, kind]: Field): string {
    if (kind === "money" && typeof value === "number") return formatMoney(value)
    if (kind === "costType" && typeof value === "string") return t(`costTypes.${value}`)
    if (kind === "billingInterval" && typeof value === "string") {
      return t(`fields.billing${value.charAt(0).toUpperCase()}${value.slice(1)}`, { defaultValue: value })
    }
    return String(value)
  }

  const problem = targetProblem(target, draft.parentId, enabledTypes, parentOptions)
  const hint = problem && t(hintKeys[problem])

  const title = draft.source.title || `#${draft.source.ref}`

  const fields = summaryFields(draft).filter(([, value]) => value !== undefined && value !== null && value !== "")

  return (
    <Card>
      <CardContent className="flex gap-4">
        <Checkbox
          className="mt-1"
          aria-label={t("drafts.bulk.selectDraft", { title })}
          checked={selected}
          disabled={busy || problem !== null}
          onCheckedChange={(checked) => onSelectedChange(checked === true)}
        />
        {Number.isInteger(documentId) && documentId > 0 && (
          <PaperlessThumbnail documentId={documentId} className="h-32 w-24 shrink-0" />
        )}
        <div className="min-w-0 flex-1 space-y-3">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate font-medium">{title}</p>
              <p className="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
                <span>{t(`drafts.sources.${draft.source.kind}`)}</span>
                <span>{formatDateTime(draft.createdAt)}</span>
                {draft.source.kind === "paperless" && draft.source.url && (
                  <a
                    href={draft.source.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-primary hover:underline"
                  >
                    <ExternalLink className="h-3 w-3" />
                    {t("paperless.openInPaperless")}
                  </a>
                )}
              </p>
            </div>
            <Badge variant={target === "none" ? "outline" : "secondary"}>{t(`drafts.types.${target}`)}</Badge>
          </div>

          {draft.note && (
            <p className="text-sm text-amber-700 dark:text-amber-400">
              {/* notes from agents are free text, the other sources send codes */}
              {draft.source.kind === "mcp" ? draft.note : t(`drafts.notes.${draft.note}`, { defaultValue: draft.note })}
            </p>
          )}

          {fields.length > 0 && (
            <dl className="grid grid-cols-1 gap-x-4 gap-y-1 text-sm sm:grid-cols-2">
              {fields.map((field) => (
                <div key={field[0]} className="flex min-w-0 gap-2">
                  <dt className="shrink-0 text-muted-foreground">{t(field[0])}:</dt>
                  <dd className="truncate">{display(field)}</dd>
                </div>
              ))}
            </dl>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <TargetPicker
              target={target}
              parentId={draft.parentId}
              enabledTypes={enabledTypes}
              parentOptions={parentOptions}
              disabled={busy}
              onChangeType={onChangeType}
              onChangeParent={onChangeParent}
            />
            <div className="ml-auto flex gap-2">
              <Button variant="outline" onClick={onReject} disabled={busy}>
                {t("drafts.reject")}
              </Button>
              <Button onClick={onAccept} disabled={busy || problem !== null}>
                {t("drafts.accept")}
              </Button>
            </div>
          </div>
          {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}
