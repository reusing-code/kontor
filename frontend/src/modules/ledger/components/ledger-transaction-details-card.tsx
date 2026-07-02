import { useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Copy, ExternalLink, Link2, Save } from "lucide-react"
import { useCategories } from "@/hooks/use-categories"
import { useModules } from "@/hooks/use-modules"
import { useContracts, useCreateContractByCategory } from "@/modules/contracts/hooks/use-contracts"
import { useLedgerCategories, useLedgerEmailOrders, useUpdateLedgerTransactionDetails } from "@/modules/ledger/hooks/use-ledger"
import { useCreatePurchaseByCategory, usePurchases } from "@/modules/purchases/hooks/use-purchases"
import { useCreateVehicle, useVehicles } from "@/modules/auto/hooks/use-vehicles"
import { moduleReferenceToPath, referenceModuleId } from "@/modules/ledger/lib/module-links"
import { formatAmountMinor, formatLedgerDate, formatLedgerReviewStatus, formatLedgerSpecialCategory } from "@/modules/ledger/lib/ledger-utils"
import type { ContractFormData } from "@/modules/contracts/types"
import type { LedgerTransaction, LedgerTransactionReference } from "@/modules/ledger/types"
import type { PurchaseFormData } from "@/modules/purchases/types"
import type { VehicleFormData } from "@/modules/auto/types"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { PurchaseDialog } from "@/modules/purchases/components/purchase-dialog"
import { VehicleDialog } from "@/modules/auto/components/vehicle-dialog"
import { LedgerTransferManager } from "@/modules/ledger/components/ledger-transfer-manager"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

type ReferenceType = LedgerTransactionReference["type"]

const REFERENCE_NAV_KEY: Record<ReferenceType, string> = {
  purchase: "nav.purchases",
  contract: "nav.contracts",
  vehicle: "nav.auto",
}

type TargetOption = { id: string; label: string }

function TargetSelect({ options, value, onChange, placeholder }: {
  options: TargetOption[]
  value: string
  onChange: (value: string) => void
  placeholder: string
}) {
  return (
    <Select value={value || undefined} onValueChange={onChange}>
      <SelectTrigger className="w-full">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

type TargetSelectProps = {
  value: string
  onChange: (value: string) => void
  placeholder: string
}

function PurchaseTargetSelect(props: TargetSelectProps) {
  const { data: purchases = [] } = usePurchases()
  return <TargetSelect options={purchases.map((purchase) => ({ id: purchase.id, label: purchase.itemName }))} {...props} />
}

function ContractTargetSelect(props: TargetSelectProps) {
  const { data: contracts = [] } = useContracts()
  return <TargetSelect options={contracts.map((contract) => ({ id: contract.id, label: contract.name }))} {...props} />
}

function VehicleTargetSelect(props: TargetSelectProps) {
  const { data: vehicles = [] } = useVehicles()
  return <TargetSelect options={vehicles.map((vehicle) => ({ id: vehicle.id, label: vehicle.name }))} {...props} />
}

function PurchaseCreateAndLink({ onLinked }: { onLinked: (targetId: string) => void }) {
  const { t } = useTranslation()
  const { data: purchaseCategories = [] } = useCategories("purchases")
  const createPurchase = useCreatePurchaseByCategory()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [categoryId, setCategoryId] = useState("")

  function handleCreate(data: PurchaseFormData) {
    if (!categoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    createPurchase.mutate({ categoryId, data }, {
      onSuccess: (purchase) => {
        onLinked(purchase.id)
        setDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.purchases") }))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <>
      <Select value={categoryId || undefined} onValueChange={setCategoryId}>
        <SelectTrigger className="w-[14rem]">
          <SelectValue placeholder={t("ledger.purchaseCategoryForCreate")} />
        </SelectTrigger>
        <SelectContent>
          {purchaseCategories.map((category) => (
            <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="secondary" onClick={() => setDialogOpen(true)}>{t("ledger.createPurchaseAndLink")}</Button>
      <PurchaseDialog open={dialogOpen} onOpenChange={setDialogOpen} onSubmit={handleCreate} />
    </>
  )
}

function ContractCreateAndLink({ onLinked }: { onLinked: (targetId: string) => void }) {
  const { t } = useTranslation()
  const { data: contractCategories = [] } = useCategories("contracts")
  const createContract = useCreateContractByCategory()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [categoryId, setCategoryId] = useState("")

  function handleCreate(data: ContractFormData) {
    if (!categoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    createContract.mutate({ categoryId, data }, {
      onSuccess: (contract) => {
        onLinked(contract.id)
        setDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.contracts") }))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <>
      <Select value={categoryId || undefined} onValueChange={setCategoryId}>
        <SelectTrigger className="w-[14rem]">
          <SelectValue placeholder={t("ledger.contractCategoryForCreate")} />
        </SelectTrigger>
        <SelectContent>
          {contractCategories.map((category) => (
            <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Button type="button" variant="secondary" onClick={() => setDialogOpen(true)}>{t("ledger.createContractAndLink")}</Button>
      <ContractDialog open={dialogOpen} onOpenChange={setDialogOpen} onSubmit={handleCreate} />
    </>
  )
}

function VehicleCreateAndLink({ onLinked }: { onLinked: (targetId: string) => void }) {
  const { t } = useTranslation()
  const createVehicle = useCreateVehicle()
  const [dialogOpen, setDialogOpen] = useState(false)

  function handleCreate(data: VehicleFormData) {
    createVehicle.mutate(data, {
      onSuccess: (vehicle) => {
        onLinked(vehicle.id)
        setDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.auto") }))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <>
      <Button type="button" variant="secondary" onClick={() => setDialogOpen(true)}>{t("ledger.createVehicleAndLink")}</Button>
      <VehicleDialog open={dialogOpen} onOpenChange={setDialogOpen} onSubmit={handleCreate} />
    </>
  )
}

function ReferenceRowShell({ reference, label, description, hint, onRemove }: {
  reference: LedgerTransactionReference
  label: string
  description?: string
  hint?: string
  onRemove: () => void
}) {
  const { t } = useTranslation()
  const linked = hint === undefined

  return (
    <div className="flex items-center gap-2 rounded-md border p-3 text-sm">
      <Link2 className="h-4 w-4 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        {linked ? (
          <Link to={moduleReferenceToPath(reference)} className="block truncate text-primary hover:underline">
            {label}
          </Link>
        ) : (
          <div className="block truncate">
            {label} <span className="text-muted-foreground">{hint}</span>
          </div>
        )}
        {description ? <div className="truncate text-xs text-muted-foreground">{description}</div> : null}
      </div>
      <Button type="button" variant="ghost" size="sm" onClick={onRemove}>
        {t("common.delete")}
      </Button>
    </div>
  )
}

function PurchaseReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: purchases = [] } = usePurchases()
  const purchase = purchases.find((item) => item.id === reference.targetId)
  const label = purchase?.itemName ?? `${t("nav.purchases")} ${reference.targetId}`
  const description = purchase ? [purchase.dealer, purchase.purchaseDate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function ContractReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: contracts = [] } = useContracts()
  const contract = contracts.find((item) => item.id === reference.targetId)
  const label = contract?.name ?? `${t("nav.contracts")} ${reference.targetId}`
  const description = contract ? [contract.company, contract.startDate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function VehicleReferenceRow({ reference, onRemove }: { reference: LedgerTransactionReference; onRemove: () => void }) {
  const { t } = useTranslation()
  const { data: vehicles = [] } = useVehicles()
  const vehicle = vehicles.find((item) => item.id === reference.targetId)
  const label = vehicle?.name ?? `${t("nav.auto")} ${reference.targetId}`
  const description = vehicle ? [vehicle.make, vehicle.model, vehicle.licensePlate].filter(Boolean).join(" • ") : undefined
  return <ReferenceRowShell reference={reference} label={label} description={description} onRemove={onRemove} />
}

function ReferenceRow({ reference, enabled, onRemove }: {
  reference: LedgerTransactionReference
  enabled: boolean
  onRemove: () => void
}) {
  const { t } = useTranslation()

  if (!enabled) {
    const label = `${t(REFERENCE_NAV_KEY[reference.type])} ${reference.targetId}`
    return <ReferenceRowShell reference={reference} label={label} hint={t("modules.disabledHint")} onRemove={onRemove} />
  }

  switch (reference.type) {
    case "purchase":
      return <PurchaseReferenceRow reference={reference} onRemove={onRemove} />
    case "contract":
      return <ContractReferenceRow reference={reference} onRemove={onRemove} />
    case "vehicle":
      return <VehicleReferenceRow reference={reference} onRemove={onRemove} />
  }
}

export function LedgerTransactionDetailsCard({ transaction }: { transaction: LedgerTransaction }) {
  const { t, i18n } = useTranslation()
  const { isEnabled } = useModules()
  const { data: ledgerCategories = [] } = useLedgerCategories()
  const { data: emailOrders = [] } = useLedgerEmailOrders()
  const updateDetails = useUpdateLedgerTransactionDetails()

  const availableTypes = (["purchase", "contract", "vehicle"] as ReferenceType[]).filter((type) =>
    isEnabled(referenceModuleId({ type, targetId: "" })),
  )

  const [note, setNote] = useState(transaction.note ?? "")
  const [linksText, setLinksText] = useState((transaction.links ?? []).join("\n"))
  const [references, setReferences] = useState<LedgerTransactionReference[]>(transaction.references ?? [])
  const [referenceType, setReferenceType] = useState<ReferenceType>("purchase")
  const [referenceTargetId, setReferenceTargetId] = useState("")

  const effectiveReferenceType = availableTypes.includes(referenceType) ? referenceType : availableTypes[0]

  const categoryName = transaction.categoryId ? ledgerCategories.find((category) => category.id === transaction.categoryId)?.name : undefined
  const linkedEmailOrders = emailOrders.filter((order) => (transaction.emailOrderIds ?? []).includes(order.id))

  function addReference(reference: LedgerTransactionReference) {
    setReferences((current) => {
      const key = `${reference.type}:${reference.targetId}`
      if (current.some((item) => `${item.type}:${item.targetId}` === key)) {
        return current
      }
      return [...current, reference]
    })
  }

  function handleAddReference() {
    if (!effectiveReferenceType || !referenceTargetId) {
      return
    }
    addReference({ type: effectiveReferenceType, targetId: referenceTargetId })
    setReferenceTargetId("")
  }

  function handleCopyLink() {
    navigator.clipboard.writeText(window.location.href)
      .then(() => toast.success(t("ledger.transactionLinkCopied")))
      .catch(() => toast.error(t("ledger.transactionLinkCopyFailed")))
  }

  function handleSave() {
    const links = linksText.split("\n").map((value) => value.trim()).filter(Boolean)
    updateDetails.mutate({ id: transaction.id, data: { note, links, references } }, {
      onSuccess: () => toast.success(t("ledger.transactionDetailsSaved")),
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between gap-3">
            <CardTitle>{t("ledger.transactionDetails")}</CardTitle>
            <Button type="button" variant="outline" size="sm" onClick={handleCopyLink}>
              <Copy className="mr-2 h-4 w-4" />
              {t("ledger.copyTransactionLink")}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.date")}</div>
            <div>{formatLedgerDate(transaction.bookingDate)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.valueDate")}</div>
            <div>{formatLedgerDate(transaction.valueDate)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.amount")}</div>
            <div className={transaction.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
              {formatAmountMinor(transaction.amountMinor, transaction.currency, i18n.language)}
            </div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.reviewState")}</div>
            <div>{formatLedgerReviewStatus(transaction.reviewStatus)}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.counterparty")}</div>
            <div>{transaction.counterpartyName || "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.category")}</div>
            <div>{categoryName ?? t("ledger.noCategory")}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.specialCategory")}</div>
            <div>{transaction.specialCategory ? formatLedgerSpecialCategory(transaction.specialCategory) : "-"}</div>
          </div>
          <div className="md:col-span-2">
            <div className="text-xs text-muted-foreground">{t("ledger.purpose")}</div>
            <div className="whitespace-pre-wrap">{transaction.purpose || "-"}</div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.internalTransfer")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <LedgerTransferManager transaction={transaction} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.additionalInformation")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <div className="text-sm font-medium">{t("ledger.email.orders")}</div>
            {linkedEmailOrders.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("ledger.email.noLinkedOrders")}</p>
            ) : (
              linkedEmailOrders.map((order) => (
                <div key={order.id} className="rounded-md border p-3 text-sm">
                  <div className="font-medium">{order.externalOrderId || order.emailSubject || order.importerId}</div>
                  <div className="text-xs text-muted-foreground">{formatLedgerDate(order.orderDate)}</div>
                  <div className="mt-2 space-y-1 text-muted-foreground">
                    {(order.items ?? []).map((item, index) => (
                      <div key={`${order.id}-${index}`}>{item.quantity}x {item.name}</div>
                    ))}
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="space-y-2">
            <div className="text-sm font-medium">{t("ledger.note")}</div>
            <Textarea value={note} onChange={(event) => setNote(event.target.value)} rows={5} />
          </div>

          <div className="space-y-2">
            <div className="text-sm font-medium">{t("ledger.links")}</div>
            <Textarea value={linksText} onChange={(event) => setLinksText(event.target.value)} rows={4} placeholder="https://example.com/document.pdf" />
            <div className="space-y-2">
              {(transaction.links ?? []).map((link) => (
                <a key={link} href={link} target="_blank" rel="noreferrer" className="flex items-center gap-2 text-sm text-primary hover:underline break-all">
                  <ExternalLink className="h-4 w-4" />
                  {link}
                </a>
              ))}
            </div>
          </div>

          <div className="space-y-3">
            <div className="text-sm font-medium">{t("ledger.crossReferences")}</div>
            {effectiveReferenceType !== undefined && (
              <>
                <div className="grid gap-2 md:grid-cols-[12rem_1fr_auto]">
                  <Select
                    value={effectiveReferenceType}
                    onValueChange={(value) => { setReferenceType(value as ReferenceType); setReferenceTargetId("") }}
                  >
                    <SelectTrigger className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {availableTypes.map((type) => (
                        <SelectItem key={type} value={type}>{t(REFERENCE_NAV_KEY[type])}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {effectiveReferenceType === "purchase" && (
                    <PurchaseTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
                  )}
                  {effectiveReferenceType === "contract" && (
                    <ContractTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
                  )}
                  {effectiveReferenceType === "vehicle" && (
                    <VehicleTargetSelect value={referenceTargetId} onChange={setReferenceTargetId} placeholder={t("ledger.selectReferenceTarget")} />
                  )}
                  <Button type="button" variant="outline" onClick={handleAddReference}>{t("ledger.addReference")}</Button>
                </div>

                <div className="flex flex-wrap gap-2">
                  {isEnabled("purchases") && (
                    <PurchaseCreateAndLink onLinked={(targetId) => addReference({ type: "purchase", targetId })} />
                  )}
                  {isEnabled("contracts") && (
                    <ContractCreateAndLink onLinked={(targetId) => addReference({ type: "contract", targetId })} />
                  )}
                  {isEnabled("auto") && (
                    <VehicleCreateAndLink onLinked={(targetId) => addReference({ type: "vehicle", targetId })} />
                  )}
                </div>
              </>
            )}

            <div className="space-y-2">
              {references.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("ledger.noReferences")}</p>
              ) : (
                references.map((reference) => (
                  <ReferenceRow
                    key={`${reference.type}:${reference.targetId}`}
                    reference={reference}
                    enabled={isEnabled(referenceModuleId(reference))}
                    onRemove={() => setReferences((current) => current.filter((item) => !(item.type === reference.type && item.targetId === reference.targetId)))}
                  />
                ))
              )}
            </div>
          </div>

          <div className="flex justify-end">
            <Button type="button" onClick={handleSave} disabled={updateDetails.isPending}>
              <Save className="mr-2 h-4 w-4" />
              {t("common.save")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
