import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { Camera, FileText, Image as ImageIcon, Loader2, Paperclip, X } from "lucide-react"
import { useCapabilities } from "@/hooks/use-capabilities"
import { useQuickAdd } from "@/hooks/use-quick-add"
import { ApiError } from "@/lib/api"
import { prepareQuickAddFile, QuickAddFileError } from "@/lib/quick-add-file"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { TargetPicker } from "@/components/drafts/target-picker"
import { useParentOptions } from "@/components/drafts/use-parent-options"
import { isClassifiedType, targetProblem, type PrefillEntry, type TargetProblem } from "@/components/drafts/draft-input"
import { quickAddMaxLength, type QuickAddCurrency, type QuickAddResponse } from "@/types/quick-add"
import type { ClassifiedEntityType, DraftTargetType } from "@/types/drafts"

// Error codes of the quick-add endpoint and of the file check that have a
// translation under quickAdd.errors.
const errorCodes = ["file_too_large", "unsupported_file", "vision_not_configured", "pdf_no_text", "file_unreadable"]

const hasTouch = typeof window !== "undefined" && window.matchMedia("(pointer: coarse)").matches

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
  enabledTypes: ClassifiedEntityType[]
  text: string
  onTextChange: (text: string) => void
  file: File | null
  onFileChange: (file: File | null) => void
  onContinue: (entry: PrefillEntry) => void
}

// QuickAddDialog sends the typed note, or an attached PDF or photo with the
// note as a hint, to the LLM. A clear proposal goes
// straight to the prefilled create dialog; otherwise the user confirms the
// type and category or vehicle first. Choosing another type asks the LLM
// again with that type.
export function QuickAddDialog({
  open,
  onOpenChange,
  enabledTypes,
  text,
  onTextChange,
  file,
  onFileChange,
  onContinue,
}: QuickAddDialogProps) {
  const { t } = useTranslation()
  const { llmVision } = useCapabilities()
  const classify = useQuickAdd()
  const fileInput = useRef<HTMLInputElement>(null)
  const cameraInput = useRef<HTMLInputElement>(null)
  const [preparing, setPreparing] = useState(false)
  const parentOptions = useParentOptions()
  const [proposal, setProposal] = useState<QuickAddResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  const trimmed = text.trim()
  const busy = classify.isPending || preparing
  const canSubmit = (trimmed !== "" || file !== null) && !busy

  function errorMessage(err: unknown): string {
    const code = err instanceof ApiError || err instanceof QuickAddFileError ? err.code : undefined
    if (code && errorCodes.includes(code)) return t(`quickAdd.errors.${code}`)
    return t("quickAdd.failed", { message: (err as Error).message })
  }

  async function handlePick(e: React.ChangeEvent<HTMLInputElement>) {
    const picked = e.target.files?.[0]
    // Lets the same file be picked again after removing it.
    e.target.value = ""
    if (!picked) return
    setError(null)
    setProposal(null)
    setPreparing(true)
    try {
      onFileChange(await prepareQuickAddFile(picked, llmVision))
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setPreparing(false)
    }
  }

  function removeFile() {
    onFileChange(null)
    setProposal(null)
    setError(null)
  }

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

  function run(targetType?: ClassifiedEntityType) {
    setError(null)
    classify.mutate(
      { text: trimmed, file: file ?? undefined, targetType },
      {
        onSuccess: (res) => {
          const problem = targetProblem(res.targetType, res.parentId, enabledTypes, parentOptions)
          if (res.confident && res.targetType !== "none" && problem === null) {
            finish({ type: res.targetType, parentId: res.parentId ?? "", input: res.input })
            return
          }
          setProposal(res)
        },
        onError: (err) => setError(errorMessage(err)),
      },
    )
  }

  function handleChangeType(type: DraftTargetType) {
    if (!proposal || type === proposal.targetType) return
    if (type === "none") {
      setProposal({ ...proposal, targetType: "none", parentId: undefined, confident: false })
      return
    }
    if (isClassifiedType(type)) run(type)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault()
      if (canSubmit) run()
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
          placeholder={file ? t("quickAdd.placeholderFile") : t("quickAdd.placeholder")}
          maxLength={quickAddMaxLength}
          disabled={busy}
          rows={3}
          aria-label={t("quickAdd.title")}
        />

        <input
          ref={fileInput}
          type="file"
          hidden
          accept={llmVision ? "application/pdf,image/*" : "application/pdf"}
          onChange={(e) => void handlePick(e)}
        />
        <input ref={cameraInput} type="file" hidden accept="image/*" capture="environment" onChange={(e) => void handlePick(e)} />
        {file ? (
          <div className="flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
            {file.type === "application/pdf" ? <FileText className="size-4 shrink-0" /> : <ImageIcon className="size-4 shrink-0" />}
            <span className="min-w-0 flex-1 truncate">{file.name}</span>
            <span className="shrink-0 text-xs text-muted-foreground">{formatSize(file.size)}</span>
            <Button variant="ghost" size="icon" className="size-6" disabled={busy} onClick={removeFile} aria-label={t("quickAdd.removeFile")}>
              <X />
            </Button>
          </div>
        ) : (
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" disabled={busy} onClick={() => fileInput.current?.click()}>
              {preparing ? <Loader2 className="animate-spin" /> : <Paperclip />}
              {t(llmVision ? "quickAdd.attach" : "quickAdd.attachPdf")}
            </Button>
            {llmVision && hasTouch && (
              <Button variant="outline" size="sm" disabled={busy} onClick={() => cameraInput.current?.click()}>
                <Camera />
                {t("quickAdd.takePhoto")}
              </Button>
            )}
          </div>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

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
            <Button disabled={!canSubmit} onClick={() => run()}>
              {classify.isPending && <Loader2 className="animate-spin" />}
              {classify.isPending ? t("quickAdd.thinking") : t("quickAdd.submit")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function formatSize(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} kB` : `${(bytes / 1024 / 1024).toFixed(1)} MB`
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
