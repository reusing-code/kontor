import { useState } from "react"
import { createRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { usePageTitle } from "@/hooks/use-page-title"
import { useContract, useUpdateContractById } from "@/modules/contracts/hooks/use-contracts"
import { LinkedTransactionsList } from "@/components/linked-transactions-list"
import { ContractDialog } from "@/modules/contracts/components/contract-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { rootRoute } from "@/routes/__root"

export const contractDetailRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/contracts/$contractId",
  component: ContractDetailPage,
})

export function ContractDetailPage() {
  const { t } = useTranslation()
  const { contractId } = contractDetailRoute.useParams()
  const { data: contract } = useContract(contractId)
  const updateContract = useUpdateContractById()
  const [editing, setEditing] = useState(false)

  usePageTitle(contract?.name ?? t("nav.contracts"), t("app.title"))

  if (!contract) {
    return <div className="text-sm text-muted-foreground">{t("common.loading")}</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-2xl font-bold">{contract.name}</h1>
        <Button variant="outline" onClick={() => setEditing(true)}>{t("common.edit")}</Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("contract.edit")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div><div className="text-xs text-muted-foreground">{t("fields.company")}</div><div>{contract.company || "-"}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("fields.price")}</div><div>{contract.price ? `${contract.price.toFixed(2)} ${t("common.currency")}` : "-"}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("fields.startDate")}</div><div>{contract.startDate}</div></div>
          <div><div className="text-xs text-muted-foreground">{t("fields.endDate")}</div><div>{contract.endDate || "-"}</div></div>
          <div className="md:col-span-2"><div className="text-xs text-muted-foreground">{t("fields.comments")}</div><div className="whitespace-pre-wrap">{contract.comments || "-"}</div></div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.linkedTransactions")}</CardTitle>
        </CardHeader>
        <CardContent>
          <LinkedTransactionsList transactionIds={contract.linkedTransactionIds ?? []} />
        </CardContent>
      </Card>

      <ContractDialog
        open={editing}
        onOpenChange={setEditing}
        contract={contract}
        onSubmit={(data) => updateContract.mutate({ id: contract.id, data }, { onSuccess: () => toast.success(t("contract.updated")) })}
      />
    </div>
  )
}
