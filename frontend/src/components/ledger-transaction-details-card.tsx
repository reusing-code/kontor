import { useMemo, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Copy, ExternalLink, Link2, Save } from "lucide-react"
import { useCategories } from "@/hooks/use-categories"
import { useContracts, useCreateContractByCategory } from "@/hooks/use-contracts"
import { useLedgerCategories, useLedgerEmailOrders, useUpdateLedgerTransactionDetails } from "@/hooks/use-ledger"
import { useCreatePurchaseByCategory, usePurchases } from "@/hooks/use-purchases"
import { useCreateVehicle, useVehicles } from "@/hooks/use-vehicles"
import { moduleReferenceToPath } from "@/lib/module-links"
import { formatAmountMinor, formatLedgerDate, formatLedgerReviewStatus, formatLedgerSpecialCategory } from "@/lib/ledger-utils"
import type { ContractFormData } from "@/types/contract"
import type { LedgerTransaction, LedgerTransactionReference } from "@/types/ledger"
import type { PurchaseFormData } from "@/types/purchase"
import type { VehicleFormData } from "@/types/vehicle"
import { ContractDialog } from "@/components/contract-dialog"
import { PurchaseDialog } from "@/components/purchase-dialog"
import { VehicleDialog } from "@/components/vehicle-dialog"
import { LedgerTransferManager } from "@/components/ledger-transfer-manager"
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

type ReferenceLookups = {
  purchaseNames: Map<string, string>
  contractNames: Map<string, string>
  vehicleNames: Map<string, string>
  purchaseDetails: Map<string, string>
  contractDetails: Map<string, string>
  vehicleDetails: Map<string, string>
}

function referenceLabel(reference: LedgerTransactionReference, lookups: ReferenceLookups, t: (key: string) => string) {
  switch (reference.type) {
    case "purchase":
      return lookups.purchaseNames.get(reference.targetId) ?? `${t("nav.purchases")} ${reference.targetId}`
    case "contract":
      return lookups.contractNames.get(reference.targetId) ?? `${t("nav.contracts")} ${reference.targetId}`
    case "vehicle":
      return lookups.vehicleNames.get(reference.targetId) ?? `${t("nav.auto")} ${reference.targetId}`
  }
}

function referenceDescription(reference: LedgerTransactionReference, lookups: ReferenceLookups) {
  switch (reference.type) {
    case "purchase":
      return lookups.purchaseDetails.get(reference.targetId)
    case "contract":
      return lookups.contractDetails.get(reference.targetId)
    case "vehicle":
      return lookups.vehicleDetails.get(reference.targetId)
  }
}

export function LedgerTransactionDetailsCard({ transaction }: { transaction: LedgerTransaction }) {
  const { t, i18n } = useTranslation()
  const { data: ledgerCategories = [] } = useLedgerCategories()
  const { data: purchaseCategories = [] } = useCategories("purchases")
  const { data: contractCategories = [] } = useCategories("contracts")
  const { data: purchases = [] } = usePurchases()
  const { data: contracts = [] } = useContracts()
  const { data: vehicles = [] } = useVehicles()
  const { data: emailOrders = [] } = useLedgerEmailOrders()
  const updateDetails = useUpdateLedgerTransactionDetails()
  const createPurchase = useCreatePurchaseByCategory()
  const createContract = useCreateContractByCategory()
  const createVehicle = useCreateVehicle()

  const [note, setNote] = useState(transaction.note ?? "")
  const [linksText, setLinksText] = useState((transaction.links ?? []).join("\n"))
  const [references, setReferences] = useState<LedgerTransactionReference[]>(transaction.references ?? [])
  const [referenceType, setReferenceType] = useState<LedgerTransactionReference["type"]>("purchase")
  const [referenceTargetId, setReferenceTargetId] = useState("")
  const [purchaseDialogOpen, setPurchaseDialogOpen] = useState(false)
  const [contractDialogOpen, setContractDialogOpen] = useState(false)
  const [vehicleDialogOpen, setVehicleDialogOpen] = useState(false)
  const [purchaseCategoryId, setPurchaseCategoryId] = useState("")
  const [contractCategoryId, setContractCategoryId] = useState("")

  const categoryName = transaction.categoryId ? ledgerCategories.find((category) => category.id === transaction.categoryId)?.name : undefined
  const purchaseNames = useMemo(() => new Map(purchases.map((purchase) => [purchase.id, purchase.itemName])), [purchases])
  const contractNames = useMemo(() => new Map(contracts.map((contract) => [contract.id, contract.name])), [contracts])
  const vehicleNames = useMemo(() => new Map(vehicles.map((vehicle) => [vehicle.id, vehicle.name])), [vehicles])
  const purchaseDetails = useMemo(() => new Map(purchases.map((purchase) => [purchase.id, [purchase.dealer, purchase.purchaseDate].filter(Boolean).join(" • ")])), [purchases])
  const contractDetails = useMemo(() => new Map(contracts.map((contract) => [contract.id, [contract.company, contract.startDate].filter(Boolean).join(" • ")])), [contracts])
  const vehicleDetails = useMemo(() => new Map(vehicles.map((vehicle) => [vehicle.id, [vehicle.make, vehicle.model, vehicle.licensePlate].filter(Boolean).join(" • ")])), [vehicles])
  const lookups = { purchaseNames, contractNames, vehicleNames, purchaseDetails, contractDetails, vehicleDetails }
  const linkedEmailOrders = emailOrders.filter((order) => (transaction.emailOrderIds ?? []).includes(order.id))

  const referenceOptions = useMemo(() => {
    switch (referenceType) {
      case "purchase":
        return purchases.map((purchase) => ({ id: purchase.id, label: purchase.itemName }))
      case "contract":
        return contracts.map((contract) => ({ id: contract.id, label: contract.name }))
      case "vehicle":
        return vehicles.map((vehicle) => ({ id: vehicle.id, label: vehicle.name }))
    }
  }, [contracts, purchases, referenceType, vehicles])

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
    if (!referenceTargetId) {
      return
    }
    addReference({ type: referenceType, targetId: referenceTargetId })
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

  function handleCreatePurchase(data: PurchaseFormData) {
    if (!purchaseCategoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    createPurchase.mutate({ categoryId: purchaseCategoryId, data }, {
      onSuccess: (purchase) => {
        addReference({ type: "purchase", targetId: purchase.id })
        setPurchaseDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.purchases") }))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleCreateContract(data: ContractFormData) {
    if (!contractCategoryId) {
      toast.error(t("ledger.selectCategoryFirst"))
      return
    }
    createContract.mutate({ categoryId: contractCategoryId, data }, {
      onSuccess: (contract) => {
        addReference({ type: "contract", targetId: contract.id })
        setContractDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.contracts") }))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  function handleCreateVehicle(data: VehicleFormData) {
    createVehicle.mutate(data, {
      onSuccess: (vehicle) => {
        addReference({ type: "vehicle", targetId: vehicle.id })
        setVehicleDialogOpen(false)
        toast.success(t("ledger.referenceCreatedAndLinked", { type: t("nav.auto") }))
      },
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
            <div className="grid gap-2 md:grid-cols-[12rem_1fr_auto]">
              <Select value={referenceType} onValueChange={(value) => { setReferenceType(value as LedgerTransactionReference["type"]); setReferenceTargetId("") }}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="purchase">{t("nav.purchases")}</SelectItem>
                  <SelectItem value="contract">{t("nav.contracts")}</SelectItem>
                  <SelectItem value="vehicle">{t("nav.auto")}</SelectItem>
                </SelectContent>
              </Select>
              <Select value={referenceTargetId || undefined} onValueChange={setReferenceTargetId}>
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={t("ledger.selectReferenceTarget")} />
                </SelectTrigger>
                <SelectContent>
                  {referenceOptions.map((option) => (
                    <SelectItem key={option.id} value={option.id}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type="button" variant="outline" onClick={handleAddReference}>{t("ledger.addReference")}</Button>
            </div>

            <div className="flex flex-wrap gap-2">
              <Select value={purchaseCategoryId || undefined} onValueChange={setPurchaseCategoryId}>
                <SelectTrigger className="w-[14rem]">
                  <SelectValue placeholder={t("ledger.purchaseCategoryForCreate")} />
                </SelectTrigger>
                <SelectContent>
                  {purchaseCategories.map((category) => (
                    <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type="button" variant="secondary" onClick={() => setPurchaseDialogOpen(true)}>{t("ledger.createPurchaseAndLink")}</Button>

              <Select value={contractCategoryId || undefined} onValueChange={setContractCategoryId}>
                <SelectTrigger className="w-[14rem]">
                  <SelectValue placeholder={t("ledger.contractCategoryForCreate")} />
                </SelectTrigger>
                <SelectContent>
                  {contractCategories.map((category) => (
                    <SelectItem key={category.id} value={category.id}>{category.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button type="button" variant="secondary" onClick={() => setContractDialogOpen(true)}>{t("ledger.createContractAndLink")}</Button>

              <Button type="button" variant="secondary" onClick={() => setVehicleDialogOpen(true)}>{t("ledger.createVehicleAndLink")}</Button>
            </div>

            <div className="space-y-2">
              {references.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("ledger.noReferences")}</p>
              ) : (
                references.map((reference) => {
                  const label = referenceLabel(reference, lookups, t)
                  const description = referenceDescription(reference, lookups)
                  return (
                    <div key={`${reference.type}:${reference.targetId}`} className="flex items-center gap-2 rounded-md border p-3 text-sm">
                      <Link2 className="h-4 w-4 text-muted-foreground" />
                      <div className="min-w-0 flex-1">
                        <Link to={moduleReferenceToPath(reference)} className="block truncate text-primary hover:underline">
                          {label}
                        </Link>
                        {description ? <div className="truncate text-xs text-muted-foreground">{description}</div> : null}
                      </div>
                      <Button type="button" variant="ghost" size="sm" onClick={() => setReferences((current) => current.filter((item) => !(item.type === reference.type && item.targetId === reference.targetId)))}>
                        {t("common.delete")}
                      </Button>
                    </div>
                  )
                })
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

      <PurchaseDialog open={purchaseDialogOpen} onOpenChange={setPurchaseDialogOpen} onSubmit={handleCreatePurchase} />
      <ContractDialog open={contractDialogOpen} onOpenChange={setContractDialogOpen} onSubmit={handleCreateContract} />
      <VehicleDialog open={vehicleDialogOpen} onOpenChange={setVehicleDialogOpen} onSubmit={handleCreateVehicle} />
    </div>
  )
}
