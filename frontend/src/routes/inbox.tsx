import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Inbox } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useModules } from "@/hooks/use-modules"
import { useAcceptDraft, useAcceptDraftsBatch, usePendingDrafts, useRejectDraft, useUpdateDraft } from "@/hooks/use-drafts"
import type { Draft, DraftBatchResult, DraftTargetType } from "@/types/drafts"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { BulkAcceptBar, type DraftTypeFilter } from "@/components/drafts/bulk-accept-bar"
import { DraftCard } from "@/components/drafts/draft-card"
import { PrefilledCreateDialogs } from "@/components/drafts/prefilled-create-dialogs"
import { useParentOptions } from "@/components/drafts/use-parent-options"
import type { DraftSourceKind } from "@/types/drafts"
import {
  convertInput,
  draftEntityTypes,
  draftTargetModule,
  entityUrl,
  parentRequired,
  targetProblem,
  type PrefillEntry,
} from "@/components/drafts/draft-input"

const typeOrder: DraftTargetType[] = [...draftEntityTypes, "none"]

// How many distinct hook warnings of a bulk accept get their own toast.
const maxBulkWarnings = 3

// what accepting linked, per source; other sources link a document
const acceptedKeys: Partial<Record<DraftSourceKind, string>> = {
  ledger: "drafts.acceptedLedger",
  email: "drafts.acceptedEmail",
}

export function InboxPage() {
  const { t } = useTranslation()
  usePageTitle(t("drafts.title"), t("app.title"))
  const { isEnabled } = useModules()
  const enabledTypes = draftEntityTypes.filter((type) => isEnabled(draftTargetModule[type]))

  const { data: drafts = [], isLoading } = usePendingDrafts(true)
  const updateDraft = useUpdateDraft()
  const acceptDraft = useAcceptDraft()
  const rejectDraft = useRejectDraft()
  const parentOptions = useParentOptions()

  const [accepting, setAccepting] = useState<Draft | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const acceptBatch = useAcceptDraftsBatch()
  const [typeFilter, setTypeFilter] = useState<DraftTypeFilter>("all")
  const [selection, setSelection] = useState<ReadonlySet<string>>(new Set())
  const [bulkFailures, setBulkFailures] = useState<Record<string, DraftBatchResult>>({})
  const [confirmingBulk, setConfirmingBulk] = useState(false)
  const [bulkProgress, setBulkProgress] = useState<{ done: number; total: number } | null>(null)

  const typeCounts = typeOrder
    .map((type): [DraftTargetType, number] => [type, drafts.filter((d) => d.targetType === type).length])
    .filter(([, count]) => count > 0)
  // A filter whose last draft is gone falls back to all types.
  const filter = typeCounts.some(([type]) => type === typeFilter) ? typeFilter : "all"
  const shown = filter === "all" ? drafts : drafts.filter((d) => d.targetType === filter)
  // Only drafts that Accept would take as they are, the same rule as the button.
  const selectable = shown.filter((d) => targetProblem(d.targetType, d.parentId, enabledTypes, parentOptions) === null)
  const selected = selectable.filter((d) => selection.has(d.id))

  function setSelected(ids: string[], on: boolean) {
    setSelection((prev) => {
      const next = new Set(prev)
      ids.forEach((id) => (on ? next.add(id) : next.delete(id)))
      return next
    })
  }

  function clearFailure(id: string) {
    setBulkFailures((prev) => {
      if (!(id in prev)) return prev
      const next = { ...prev }
      delete next[id]
      return next
    })
  }

  function failureText(draft: Draft): string | undefined {
    const failure = bulkFailures[draft.id]
    return failure && t(`drafts.bulk.errors.${failure.code ?? "internal_error"}`, { message: failure.error ?? "" })
  }

  async function runBulkAccept() {
    const ids = selected.map((d) => d.id)
    setBulkProgress({ done: 0, total: ids.length })
    try {
      const { results, error } = await acceptBatch.mutateAsync({
        ids,
        onProgress: (done) => setBulkProgress({ done, total: ids.length }),
      })
      const failed = results.filter((r) => r.status === "failed")
      const accepted = results.length - failed.length
      setBulkFailures((prev) => {
        const next = { ...prev }
        results.forEach((r) => delete next[r.id])
        failed.forEach((r) => (next[r.id] = r))
        return next
      })
      // Failed and unprocessed drafts stay selected for the next attempt.
      setSelected(results.filter((r) => r.status !== "failed").map((r) => r.id), false)

      if (accepted > 0) toast.success(t("drafts.bulk.accepted", { count: accepted }))
      if (failed.length > 0) toast.error(t("drafts.bulk.failed", { count: failed.length }))
      if (error) toast.error(t("drafts.bulk.requestFailed", { message: error.message }))
      const warnings = [...new Set(results.flatMap((r) => r.warnings ?? []))]
      warnings.slice(0, maxBulkWarnings).forEach((w) => toast.warning(w))
      if (warnings.length > maxBulkWarnings) {
        toast.warning(t("drafts.bulk.moreWarnings", { count: warnings.length - maxBulkWarnings }))
      }
    } finally {
      setBulkProgress(null)
    }
  }

  const acceptEntry = useMemo<PrefillEntry | null>(
    () =>
      accepting && accepting.targetType !== "none" && (!parentRequired(accepting.targetType) || accepting.parentId)
        ? { type: accepting.targetType, parentId: accepting.parentId ?? "", input: accepting.input }
        : null,
    [accepting],
  )

  function update(draft: Draft, targetType: DraftTargetType, parentId: string, input: Record<string, unknown>) {
    setBusyId(draft.id)
    clearFailure(draft.id)
    updateDraft.mutate(
      { id: draft.id, input: { targetType, parentId, input } },
      {
        onSettled: () => setBusyId(null),
      },
    )
  }

  function handleChangeType(draft: Draft, type: DraftTargetType) {
    if (type === draft.targetType) return
    update(draft, type, "", convertInput(draft.targetType, type, draft.input))
  }

  function handleReject(draft: Draft) {
    setBusyId(draft.id)
    rejectDraft.mutate(draft.id, {
      onSuccess: (res) => {
        toast.success(t("drafts.rejected"))
        res.warnings.forEach((w) => toast.warning(w))
      },
      onSettled: () => setBusyId(null),
    })
  }

  // The entity is created through the module's regular endpoint first; only
  // then is the draft accepted, which links the source document. A failed
  // create rejects, so the create dialog stays open; a failed accept does not,
  // since the entity exists.
  async function finishAccept(draft: Draft, { type, parentId }: PrefillEntry, create: () => Promise<{ id: string }>) {
    setBusyId(draft.id)
    try {
      const entityId = (await create()).id
      try {
        const res = await acceptDraft.mutateAsync({
          id: draft.id,
          input: { entityType: type, entityId, entityUrl: entityUrl(type, entityId, parentId) },
        })
        toast.success(t(acceptedKeys[draft.source.kind] ?? "drafts.accepted"))
        res.warnings.forEach((w) => toast.warning(w))
      } catch {
        // the mutation's error toast reports it
      }
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold">{t("drafts.title")}</h1>
        <p className="text-sm text-muted-foreground">{t("drafts.description")}</p>
      </div>

      {!isLoading && drafts.length === 0 && (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed p-10 text-center text-muted-foreground">
          <Inbox className="h-8 w-8" />
          <p className="text-sm">{t("drafts.empty")}</p>
        </div>
      )}

      {drafts.length > 0 && (
        <BulkAcceptBar
          typeCounts={typeCounts}
          total={drafts.length}
          filter={filter}
          onFilterChange={(next) => {
            // a selection hidden by the filter would be accepted unseen
            setSelection(new Set())
            setTypeFilter(next)
          }}
          selectableCount={selectable.length}
          selectedCount={selected.length}
          onSelectAll={(on) => setSelected(selectable.map((d) => d.id), on)}
          onAccept={() => setConfirmingBulk(true)}
          progress={bulkProgress}
        />
      )}

      <div className="space-y-4">
        {shown.map((draft) => (
          <DraftCard
            key={draft.id}
            draft={draft}
            enabledTypes={enabledTypes}
            parentOptions={parentOptions}
            busy={busyId === draft.id || bulkProgress !== null}
            selected={selected.includes(draft)}
            onSelectedChange={(on) => setSelected([draft.id], on)}
            error={failureText(draft)}
            onChangeType={(type) => handleChangeType(draft, type)}
            onChangeParent={(parentId) => update(draft, draft.targetType, parentId, draft.input)}
            onAccept={() => setAccepting(draft)}
            onReject={() => handleReject(draft)}
          />
        ))}
      </div>

      <AlertDialog open={confirmingBulk} onOpenChange={setConfirmingBulk}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("drafts.bulk.confirmTitle", { count: selected.length })}</AlertDialogTitle>
            <AlertDialogDescription>{t("drafts.bulk.confirmDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void runBulkAccept()}>{t("drafts.bulk.confirm")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <PrefilledCreateDialogs
        entry={acceptEntry}
        onClose={() => setAccepting(null)}
        onCreate={(entry, create) => (accepting ? finishAccept(accepting, entry, create) : Promise.resolve())}
      />
    </div>
  )
}
