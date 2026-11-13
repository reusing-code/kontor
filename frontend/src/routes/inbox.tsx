import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Inbox } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useModules } from "@/hooks/use-modules"
import { useAcceptDraft, usePendingDrafts, useRejectDraft, useUpdateDraft } from "@/hooks/use-drafts"
import type { Draft, DraftTargetType } from "@/types/drafts"
import { DraftCard } from "@/components/drafts/draft-card"
import { PrefilledCreateDialogs } from "@/components/drafts/prefilled-create-dialogs"
import { useParentOptions } from "@/components/drafts/use-parent-options"
import { convertInput, draftEntityTypes, draftTargetModule, entityUrl, parentRequired, type PrefillEntry } from "@/components/drafts/draft-input"

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

  const acceptEntry = useMemo<PrefillEntry | null>(
    () =>
      accepting && accepting.targetType !== "none" && (!parentRequired(accepting.targetType) || accepting.parentId)
        ? { type: accepting.targetType, parentId: accepting.parentId ?? "", input: accepting.input }
        : null,
    [accepting],
  )

  function update(draft: Draft, targetType: DraftTargetType, parentId: string, input: Record<string, unknown>) {
    setBusyId(draft.id)
    updateDraft.mutate(
      { id: draft.id, input: { targetType, parentId, input } },
      {
        onError: (err) => toast.error(err.message),
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
      onError: (err) => toast.error(err.message),
      onSettled: () => setBusyId(null),
    })
  }

  // The entity is created through the module's regular endpoint first; only
  // then is the draft accepted, which links the source document.
  async function finishAccept(draft: Draft, { type, parentId }: PrefillEntry, create: () => Promise<{ id: string }>) {
    setBusyId(draft.id)
    try {
      let entityId: string
      try {
        entityId = (await create()).id
      } catch (err) {
        toast.error(t("drafts.createFailed", { message: (err as Error).message }))
        return
      }
      try {
        const res = await acceptDraft.mutateAsync({
          id: draft.id,
          input: { entityType: type, entityId, entityUrl: entityUrl(type, entityId, parentId) },
        })
        toast.success(t(draft.source.kind === "ledger" ? "drafts.acceptedLedger" : "drafts.accepted"))
        res.warnings.forEach((w) => toast.warning(w))
      } catch (err) {
        toast.error(t("drafts.acceptFailed", { message: (err as Error).message }))
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

      <div className="space-y-4">
        {drafts.map((draft) => (
          <DraftCard
            key={draft.id}
            draft={draft}
            enabledTypes={enabledTypes}
            parentOptions={parentOptions}
            busy={busyId === draft.id}
            onChangeType={(type) => handleChangeType(draft, type)}
            onChangeParent={(parentId) => update(draft, draft.targetType, parentId, draft.input)}
            onAccept={() => setAccepting(draft)}
            onReject={() => handleReject(draft)}
          />
        ))}
      </div>

      <PrefilledCreateDialogs
        entry={acceptEntry}
        onClose={() => setAccepting(null)}
        onCreate={(entry, create) => {
          if (accepting) void finishAccept(accepting, entry, create)
        }}
      />
    </div>
  )
}
