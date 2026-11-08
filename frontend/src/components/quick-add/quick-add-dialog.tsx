import { useState } from "react"
import { useTranslation } from "react-i18next"
import { Loader2 } from "lucide-react"
import { useQuickAdd } from "@/hooks/use-quick-add"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { TargetPicker } from "@/components/drafts/target-picker"
import { useParentOptions } from "@/components/drafts/use-parent-options"
import { targetProblem, type PrefillEntry, type TargetProblem } from "@/components/drafts/draft-input"
import { quickAddMaxLength, type QuickAddCurrency, type QuickAddResponse } from "@/types/quick-add"
import type { DraftEntityType, DraftTargetType } from "@/types/drafts"

const hintKeys: Record<TargetProblem, string> = {
  pickType: "quickAdd.hintPickType",
  moduleDisabled: "quickAdd.hintPickType",
  noCategories: "drafts.hintNoCategories",
  noVehicles: "drafts.hintNoVehicles",
  pickCategory: "quickAdd.hintPickCategory",
  pickVehicle: "quickAdd.hintPickVehicle",
}

interface QuickAddDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  enabledTypes: DraftEntityType[]
  text: string
  onTextChange: (text: string) => void
  onContinue: (entry: PrefillEntry) => void
}

// QuickAddDialog sends the typed note to the LLM. A clear proposal goes
// straight to the prefilled create dialog; otherwise the user confirms the
// type and category or vehicle first. Choosing another type asks the LLM
// again with that type.
export function QuickAddDialog({ open, onOpenChange, enabledTypes, text, onTextChange, onContinue }: QuickAddDialogProps) {
  const { t } = useTranslation()
  const classify = useQuickAdd()
  const parentOptions = useParentOptions()
  const [proposal, setProposal] = useState<QuickAddResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const trimmed = text.trim()
  const busy = classify.isPending

  function reset() {
    setProposal(null)
    setError(null)
    classify.reset()
  }

  function handleOpenChange(next: boolean) {
    if (!next) reset()
    onOpenChange(next)
  }

  function finish(entry: PrefillEntry) {
    reset()
    onContinue(entry)
  }

  function run(targetType?: DraftEntityType) {
    setError(null)
    classify.mutate(
      { text: trimmed, targetType },
      {
        onSuccess: (res) => {
          const problem = targetProblem(res.targetType, res.parentId, enabledTypes, parentOptions)
          if (res.confident && res.targetType !== "none" && problem === null) {
            finish({ type: res.targetType, parentId: res.parentId ?? "", input: res.input })
            return
          }
          setProposal(res)
        },
        onError: (err) => setError(err.message),
      },
    )
  }

  function handleChangeType(type: DraftTargetType) {
    if (!proposal || type === proposal.targetType) return
    if (type === "none") {
      setProposal({ ...proposal, targetType: "none", parentId: undefined, confident: false })
      return
    }
    run(type)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      if (trimmed && !busy) run()
    }
  }

  const problem = proposal && targetProblem(proposal.targetType, proposal.parentId, enabledTypes, parentOptions)

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("quickAdd.title")}</DialogTitle>
          <DialogDescription>{t("quickAdd.description")}</DialogDescription>
        </DialogHeader>

        <Textarea
          autoFocus
          value={text}
          onChange={(e) => {
            onTextChange(e.target.value)
            if (proposal) setProposal(null)
          }}
          onKeyDown={handleKeyDown}
          placeholder={t("quickAdd.placeholder")}
          maxLength={quickAddMaxLength}
          disabled={busy}
          rows={3}
          aria-label={t("quickAdd.title")}
        />

        {error && <p className="text-sm text-destructive">{t("quickAdd.failed", { message: error })}</p>}

        {proposal && (
          <div className="space-y-3 rounded-md border p-3">
            {proposal.currency && <CurrencyHint currency={proposal.currency} />}
            {proposal.note && !proposal.currency && (
              <p className="text-sm text-amber-700 dark:text-amber-400">
                {t(`quickAdd.notes.${proposal.note}`, { defaultValue: t("quickAdd.notes.unclassified") })}
              </p>
            )}
            <div className="flex flex-wrap items-center gap-2">
              <TargetPicker
                target={proposal.targetType}
                parentId={proposal.parentId}
                enabledTypes={enabledTypes}
                parentOptions={parentOptions}
                disabled={busy}
                onChangeType={handleChangeType}
                onChangeParent={(parentId) => setProposal({ ...proposal, parentId })}
              />
            </div>
            <p className="text-xs text-muted-foreground">
              {problem ? t(hintKeys[problem]) : t("quickAdd.hintCheck")}
            </p>
          </div>
        )}

        <DialogFooter>
          {proposal ? (
            <Button
              disabled={busy || problem !== null}
              onClick={() => {
                if (proposal.targetType !== "none") {
                  finish({ type: proposal.targetType, parentId: proposal.parentId ?? "", input: proposal.input })
                }
              }}
            >
              {busy && <Loader2 className="animate-spin" />}
              {t("quickAdd.continue")}
            </Button>
          ) : (
            <Button disabled={busy || !trimmed} onClick={() => run()}>
              {busy && <Loader2 className="animate-spin" />}
              {busy ? t("quickAdd.thinking") : t("quickAdd.submit")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CurrencyHint({ currency }: { currency: QuickAddCurrency }) {
  const { t, i18n } = useTranslation()
  const original = new Intl.NumberFormat(i18n.language, { style: "currency", currency: currency.code === "???" ? "XXX" : currency.code })
    .format(currency.amount)
  return (
    <p className="text-sm text-amber-700 dark:text-amber-400">
      {currency.converted
        ? t("quickAdd.currencyConverted", { original, date: currency.rateDate, rate: currency.rate, code: currency.code })
        : t("quickAdd.currencyNotConverted", { original })}
    </p>
  )
}
