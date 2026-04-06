import { useMemo, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ExternalLink, Link2, Save } from "lucide-react"
import { useContracts } from "@/hooks/use-contracts"
import { useLedgerCategories, useUpdateLedgerTransactionDetails } from "@/hooks/use-ledger"
import { usePurchases } from "@/hooks/use-purchases"
import { useVehicles } from "@/hooks/use-vehicles"
import { moduleReferenceToPath } from "@/lib/module-links"
import { formatAmountMinor, formatLedgerDate, formatLedgerReviewStatus } from "@/lib/ledger-utils"
import type { LedgerTransaction, LedgerTransactionReference } from "@/types/ledger"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

function referenceLabel(reference: LedgerTransactionReference, lookups: {
  purchaseNames: Map<string, string>
  contractNames: Map<string, string>
  vehicleNames: Map<string, string>
}, t: (key: string) => string) {
  switch (reference.type) {
    case "purchase":
      return lookups.purchaseNames.get(reference.targetId) ?? `${t("nav.purchases")} ${reference.targetId}`
    case "contract":
      return lookups.contractNames.get(reference.targetId) ?? `${t("nav.contracts")} ${reference.targetId}`
    case "vehicle":
      return lookups.vehicleNames.get(reference.targetId) ?? `${t("nav.auto")} ${reference.targetId}`
  }
}

export function LedgerTransactionDetailsCard({ transaction }: { transaction: LedgerTransaction }) {
  const { t, i18n } = useTranslation()
  const { data: categories = [] } = useLedgerCategories()
  const { data: purchases = [] } = usePurchases()
  const { data: contracts = [] } = useContracts()
  const { data: vehicles = [] } = useVehicles()
  const updateDetails = useUpdateLedgerTransactionDetails()

  const [note, setNote] = useState(transaction.note ?? "")
  const [linksText, setLinksText] = useState((transaction.links ?? []).join("\n"))
  const [references, setReferences] = useState<LedgerTransactionReference[]>(transaction.references ?? [])
  const [referenceType, setReferenceType] = useState<LedgerTransactionReference["type"]>("purchase")
  const [referenceTargetId, setReferenceTargetId] = useState("")

  const categoryName = transaction.categoryId ? categories.find((category) => category.id === transaction.categoryId)?.name : undefined
  const purchaseNames = useMemo(() => new Map(purchases.map((purchase) => [purchase.id, purchase.itemName])), [purchases])
  const contractNames = useMemo(() => new Map(contracts.map((contract) => [contract.id, contract.name])), [contracts])
  const vehicleNames = useMemo(() => new Map(vehicles.map((vehicle) => [vehicle.id, vehicle.name])), [vehicles])

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

  function handleAddReference() {
    if (!referenceTargetId) {
      return
    }
    const next = [...references, { type: referenceType, targetId: referenceTargetId }]
    const seen = new Set<string>()
    setReferences(next.filter((reference) => {
      const key = `${reference.type}:${reference.targetId}`
      if (seen.has(key)) {
        return false
      }
      seen.add(key)
      return true
    }))
    setReferenceTargetId("")
  }

  function handleSave() {
    const links = linksText.split("\n").map((value) => value.trim()).filter(Boolean)
    updateDetails.mutate({ id: transaction.id, data: { note, links, references } }, {
      onSuccess: () => {
        toast.success(t("ledger.transactionDetailsSaved"))
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.transactionDetails")}</CardTitle>
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
          <div className="md:col-span-2">
            <div className="text-xs text-muted-foreground">{t("ledger.purpose")}</div>
            <div className="whitespace-pre-wrap">{transaction.purpose || "-"}</div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.additionalInformation")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
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

            <div className="space-y-2">
              {references.length === 0 ? (
                <p className="text-sm text-muted-foreground">{t("ledger.noReferences")}</p>
              ) : (
                references.map((reference) => {
                  const label = referenceLabel(reference, { purchaseNames, contractNames, vehicleNames }, t)
                  return (
                    <div key={`${reference.type}:${reference.targetId}`} className="flex items-center gap-2 rounded-md border p-3 text-sm">
                      <Link2 className="h-4 w-4 text-muted-foreground" />
                      <Link to={moduleReferenceToPath(reference)} className="min-w-0 flex-1 truncate text-primary hover:underline">
                        {label}
                      </Link>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() => setReferences((current) => current.filter((item) => !(item.type === reference.type && item.targetId === reference.targetId)))}
                      >
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
    </div>
  )
}
