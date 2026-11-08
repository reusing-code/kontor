import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { useQuery } from "@tanstack/react-query"
import { toast } from "sonner"
import { Inbox } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useModules } from "@/hooks/use-modules"
import { useAcceptDraft, usePendingDrafts, useRejectDraft, useUpdateDraft } from "@/hooks/use-drafts"
import { getAllCategories } from "@/lib/category-repository"
import { getAllVehicles } from "@/modules/auto/lib/vehicle-repository"
import { useCreatePurchaseByCategory } from "@/modules/purchases/hooks/use-purchases"
import { useCreateContractByCategory } from "@/modules/contracts/hooks/use-contracts"
import { useCreateCostEntryByVehicle } from "@/modules/auto/hooks/use-vehicles"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { CostEntryDialog } from "@/modules/auto/components/cost-entry-dialog"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { ContractFormData } from "@/modules/contracts/types"
import type { CostEntryFormData } from "@/modules/auto/types"
import type { Draft, DraftEntityType, DraftTargetType } from "@/types/drafts"
import { DraftCard, type DraftOption } from "@/components/drafts/draft-card"
import { convertInput, draftEntityTypes, draftTargetModule, entityUrl, initialValues } from "@/components/drafts/draft-input"

export function InboxPage() {
  const { t } = useTranslation()
  usePageTitle(t("drafts.title"), t("app.title"))
  const { isEnabled } = useModules()
  const enabledTypes = draftEntityTypes.filter((type) => isEnabled(draftTargetModule[type]))

  const { data: drafts = [], isLoading } = usePendingDrafts(true)
  const updateDraft = useUpdateDraft()
  const acceptDraft = useAcceptDraft()
  const rejectDraft = useRejectDraft()
  const createPurchase = useCreatePurchaseByCategory()
  const createContract = useCreateContractByCategory()
  const createCost = useCreateCostEntryByVehicle()

  const purchaseCategories = useQuery({
    queryKey: ["categories", "purchases"],
    queryFn: () => getAllCategories("purchases"),
    enabled: isEnabled("purchases"),
  })
  const contractCategories = useQuery({
    queryKey: ["categories", "contracts"],
    queryFn: () => getAllCategories("contracts"),
    enabled: isEnabled("contracts"),
  })
  const vehicles = useQuery({ queryKey: ["vehicles"], queryFn: getAllVehicles, enabled: isEnabled("auto") })

  const parentOptions = useMemo<Record<DraftEntityType, DraftOption[]>>(() => {
    const categoryOptions = (cats: typeof purchaseCategories.data) =>
      (cats ?? []).map((c) => ({ id: c.id, label: c.nameKey ? t(c.nameKey) : c.name }))
    return {
      purchase: categoryOptions(purchaseCategories.data),
      contract: categoryOptions(contractCategories.data),
      cost: (vehicles.data ?? []).map((v) => ({ id: v.id, label: v.name })),
    }
  }, [purchaseCategories.data, contractCategories.data, vehicles.data, t])

  const [accepting, setAccepting] = useState<Draft | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)

  const purchaseInitial = useMemo(
    () => (accepting?.targetType === "purchase" ? initialValues<PurchaseFormData>("purchase", accepting.input) : undefined),
    [accepting],
  )
  const contractInitial = useMemo(
    () => (accepting?.targetType === "contract" ? initialValues<ContractFormData>("contract", accepting.input) : undefined),
    [accepting],
  )
  const costInitial = useMemo(
    () => (accepting?.targetType === "cost" ? initialValues<CostEntryFormData>("cost", accepting.input) : undefined),
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
  async function finishAccept(draft: Draft, create: () => Promise<{ id: string }>) {
    if (draft.targetType === "none" || !draft.parentId) return
    const type = draft.targetType
    const parentId = draft.parentId
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
        toast.success(t("drafts.accepted"))
        res.warnings.forEach((w) => toast.warning(w))
      } catch (err) {
        toast.error(t("drafts.acceptFailed", { message: (err as Error).message }))
      }
    } finally {
      setBusyId(null)
    }
  }

  function closeDialog(open: boolean) {
    if (!open) setAccepting(null)
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

      <PurchaseDialog
        open={accepting?.targetType === "purchase"}
        onOpenChange={closeDialog}
        initialValues={purchaseInitial}
        onSubmit={(data) => {
          if (accepting) void finishAccept(accepting, () => createPurchase.mutateAsync({ categoryId: accepting.parentId ?? "", data }))
        }}
      />
      <ContractDialog
        open={accepting?.targetType === "contract"}
        onOpenChange={closeDialog}
        initialValues={contractInitial}
        onSubmit={(data) => {
          if (accepting) void finishAccept(accepting, () => createContract.mutateAsync({ categoryId: accepting.parentId ?? "", data }))
        }}
      />
      <CostEntryDialog
        open={accepting?.targetType === "cost"}
        onOpenChange={closeDialog}
        initialValues={costInitial}
        onSubmit={(data) => {
          if (accepting) void finishAccept(accepting, () => createCost.mutateAsync({ vehicleId: accepting.parentId ?? "", data }))
        }}
      />
    </div>
  )
}
