import { useMemo, useState } from "react"
import { formatDate } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ExternalLink, Save } from "lucide-react"
import { useModules } from "@/hooks/use-modules"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { useLedgerEmailOrders } from "@/modules/ledger/hooks/use-ledger-email"
import { useUpdateLedgerTransactionDetails } from "@/modules/ledger/hooks/use-ledger-transactions"
import { LedgerSplitEditor } from "@/modules/ledger/components/ledger-split-editor"
import { referenceModuleId, type ReferenceType } from "@/modules/ledger/lib/module-links"
import { emailOrderTitle } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerTransaction, LedgerTransactionReference } from "@/modules/ledger/types"
import { LedgerTransferManager } from "@/modules/ledger/components/ledger-transfer-manager"
import { LedgerTransactionFieldsCard } from "@/modules/ledger/components/ledger-transaction-fields-card"
import { LedgerTransactionCreateSection } from "@/modules/ledger/components/ledger-transaction-create-section"
import { LedgerTransactionReferencePicker } from "@/modules/ledger/components/ledger-transaction-reference-picker"
import { ReferenceRow } from "@/modules/ledger/components/ledger-transaction-reference-rows"
import { useTransactionLinker } from "@/modules/ledger/hooks/use-transaction-linker"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Textarea } from "@/components/ui/textarea"

export function LedgerTransactionDetailsCard({ transaction }: { transaction: LedgerTransaction }) {
  const { t } = useTranslation()
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

  const linkedEmailOrders = useMemo(
    () => emailOrders.filter((order) => (transaction.emailOrderIds ?? []).includes(order.id)),
    [emailOrders, transaction.emailOrderIds],
  )
  const canCreate = transaction.specialCategory !== "internalTransfer"

  function addReference(reference: LedgerTransactionReference) {
    setReferences((current) => {
      const key = `${reference.type}:${reference.targetId}`
      if (current.some((item) => `${item.type}:${item.targetId}` === key)) {
        return current
      }
      return [...current, reference]
    })
  }

  const link = useTransactionLinker(transaction.id, addReference)

  function handleSave() {
    const links = linksText.split("\n").map((value) => value.trim()).filter(Boolean)
    updateDetails.mutate({ id: transaction.id, data: { note, links, references } }, {
      onSuccess: () => toast.success(t("ledger.transactionDetailsSaved")),
    })
  }

  return (
    <div className="space-y-6">
      <LedgerTransactionFieldsCard transaction={transaction} categories={ledgerCategories} />

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.split.title")}</CardTitle>
        </CardHeader>
        <CardContent>
          <LedgerSplitEditor key={transaction.id} transaction={transaction} categories={ledgerCategories} />
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
                  <div className="font-medium">{emailOrderTitle(order)}</div>
                  <div className="text-xs text-muted-foreground">{formatDate(order.orderDate)}</div>
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
            <LedgerTransactionReferencePicker availableTypes={availableTypes} onAdd={addReference} />
            {availableTypes.length > 0 && canCreate && (
              <LedgerTransactionCreateSection transaction={transaction} orders={linkedEmailOrders} link={link} />
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
