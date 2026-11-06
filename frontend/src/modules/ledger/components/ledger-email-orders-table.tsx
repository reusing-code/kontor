import { useMemo, useState } from "react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { useLedgerEmailAccounts, useLinkLedgerEmailOrder, useRejectLedgerEmailOrder } from "@/modules/ledger/hooks/use-ledger"
import { formatAmountMinor, formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerEmailOrder } from "@/modules/ledger/types"
import { LedgerEmailOrderLinkDialog } from "@/modules/ledger/components/ledger-email-order-link-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

export function LedgerEmailOrdersTable({ orders }: { orders: LedgerEmailOrder[] }) {
  const { t, i18n } = useTranslation()
  const { data: emailAccounts = [] } = useLedgerEmailAccounts()
  const emailAccountNames = useMemo(() => new Map(emailAccounts.map((account) => [account.id, account.name])), [emailAccounts])
  const linkOrder = useLinkLedgerEmailOrder()
  const rejectOrder = useRejectLedgerEmailOrder()
  const [selectedOrder, setSelectedOrder] = useState<LedgerEmailOrder | null>(null)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.email.orders")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {orders.length === 0 ? <p className="text-sm text-muted-foreground">{t("ledger.email.noOrders")}</p> : null}
        {orders.map((order) => {
          return (
            <div key={order.id} className="rounded-lg border p-4 space-y-3">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="font-medium">{order.externalOrderId || order.emailSubject || order.importerId}</div>
                  <div className="text-sm text-muted-foreground">{emailAccountNames.get(order.emailAccountId) ?? order.importerId} • {formatLedgerDate(order.orderDate)}</div>
                </div>
                <div className="text-right">
                  <div className="font-medium">{formatAmountMinor(order.totalMinor, order.currency, i18n.language)}</div>
                  <Badge variant={order.matchStatus === "matched" ? "default" : order.matchStatus === "rejected" ? "secondary" : "outline"}>{t(`ledger.email.status.${order.matchStatus}`)}</Badge>
                </div>
              </div>
              <div className="space-y-1">
                {(order.items ?? []).map((item, index) => (
                  <div key={`${order.id}-${index}`} className="text-sm text-muted-foreground">{item.quantity}x {item.name}</div>
                ))}
              </div>
              <div className="flex flex-wrap gap-2">
                {(order.linkedTransactionIds ?? []).map((transactionId) => (
                  <Button key={transactionId} type="button" variant="outline" asChild>
                    <Link to="/ledger/transactions/$transactionId" params={{ transactionId }}>{t("ledger.transaction")}</Link>
                  </Button>
                ))}
                {order.matchStatus !== "rejected" ? (
                  <Button type="button" variant="outline" onClick={() => rejectOrder.mutate(order.id, { onSuccess: () => toast.success(t("ledger.email.orderRejected")), onError: (error) => toast.error(error.message) })}>{t("ledger.email.reject")}</Button>
                ) : null}
                {order.linkedTransactionIds?.length ? null : <Button type="button" variant="outline" onClick={() => setSelectedOrder(order)}>{t("ledger.email.review")}</Button>}
              </div>
            </div>
          )
        })}
      </CardContent>
      <LedgerEmailOrderLinkDialog
        key={selectedOrder?.id ?? ""}
        open={selectedOrder !== null}
        onOpenChange={(open) => { if (!open) setSelectedOrder(null) }}
        order={selectedOrder}
        onConfirm={(transactionIds) => {
          if (!selectedOrder) {
            return
          }
          linkOrder.mutate({ id: selectedOrder.id, data: { transactionIds } }, {
            onSuccess: () => {
              toast.success(t("ledger.email.orderLinked"))
              setSelectedOrder(null)
            },
            onError: (error) => toast.error(error.message),
          })
        }}
      />
    </Card>
  )
}

