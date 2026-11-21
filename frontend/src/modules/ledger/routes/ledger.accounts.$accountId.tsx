import { useState } from "react"
import { getRouteApi, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Pencil, Trash2 } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import {
  useDeleteLedgerAccount,
  useLedgerAccount,
  useLedgerCategories,
  useLedgerTransactions,
  useUpdateLedgerAccount,
} from "@/modules/ledger/hooks/use-ledger"
import { formatLedgerDate } from "@/modules/ledger/lib/ledger-utils"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"
import { LedgerAccountEditDialog } from "@/modules/ledger/components/ledger-account-edit-dialog"
import { LedgerAccountDeleteDialog } from "@/modules/ledger/components/ledger-account-delete-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

const routeApi = getRouteApi("/ledger/accounts/$accountId")

export function LedgerAccountPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { accountId } = routeApi.useParams()
  const { data: account } = useLedgerAccount(accountId)
  const { data: categories = [] } = useLedgerCategories()
  const [cursorStack, setCursorStack] = useState<string[]>([])
  const cursor = cursorStack.length > 0 ? cursorStack[cursorStack.length - 1] : undefined
  const { data: page, isFetching } = useLedgerTransactions(accountId, 100, cursor)
  const updateAccount = useUpdateLedgerAccount()
  const deleteAccount = useDeleteLedgerAccount()
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)

  usePageTitle(account?.name ?? t("nav.ledger"), t("app.title"))

  function handleDelete() {
    if (!account) return
    deleteAccount.mutate(account.id, {
      onSuccess: () => {
        setDeleting(false)
        toast.success(t("ledger.accountDelete.done", { name: account.name }))
        void navigate({ to: "/ledger" })
      },
      onError: (error) => toast.error(error.message),
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">{account?.name ?? "..."}</h1>
          <p className="text-sm text-muted-foreground">{account?.bank}</p>
        </div>
        {account && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={() => setEditing(true)}>
              <Pencil className="mr-2 h-4 w-4" />
              {t("ledger.accountEdit.button")}
            </Button>
            <Button type="button" variant="outline" onClick={() => setDeleting(true)}>
              <Trash2 className="mr-2 h-4 w-4" />
              {t("ledger.accountDelete.button")}
            </Button>
          </div>
        )}
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("ledger.accountDetails")}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-4">
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.bank")}</div>
            <div className="text-sm">{account?.bank ?? "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.iban")}</div>
            <div className="text-sm">{account?.iban ?? t("ledger.noIban")}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.currency")}</div>
            <div className="text-sm">{account?.currency ?? "-"}</div>
          </div>
          <div>
            <div className="text-xs text-muted-foreground">{t("ledger.createdAt")}</div>
            <div className="text-sm">{formatLedgerDate(account?.createdAt)}</div>
          </div>
        </CardContent>
      </Card>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("ledger.transactions")}</h2>
        <LedgerTransactionsTable
          transactions={page?.items ?? []}
          categories={categories}
          nextCursor={page?.nextCursor}
          loadingMore={isFetching}
          onLoadMore={page?.nextCursor ? () => setCursorStack((prev) => [...prev, page.nextCursor as string]) : undefined}
        />
      </div>

      {account && (
        <>
          <LedgerAccountEditDialog
            account={account}
            open={editing}
            onOpenChange={setEditing}
            pending={updateAccount.isPending}
            onSubmit={(data) =>
              updateAccount.mutate({ id: account.id, data }, {
                onSuccess: () => {
                  setEditing(false)
                  toast.success(t("ledger.accountEdit.saved"))
                },
                onError: (error) => toast.error(error.message),
              })
            }
          />
          <LedgerAccountDeleteDialog
            account={account}
            open={deleting}
            onOpenChange={setDeleting}
            pending={deleteAccount.isPending}
            onConfirm={handleDelete}
          />
        </>
      )}
    </div>
  )
}
