import { useCallback, useState } from "react"
import { formatDate } from "@/lib/format"
import { getRouteApi, useNavigate } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Pencil, Trash2 } from "lucide-react"
import { usePageTitle } from "@/hooks/use-page-title"
import { useDeleteLedgerAccount, useLedgerAccount, useUpdateLedgerAccount } from "@/modules/ledger/hooks/use-ledger-accounts"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { LedgerAccountTransactions } from "@/modules/ledger/components/ledger-account-transactions"
import { ledgerAccountSearch } from "@/modules/ledger/lib/account-search"
import { LedgerAccountEditDialog } from "@/modules/ledger/components/ledger-account-edit-dialog"
import { LedgerAccountDeleteDialog } from "@/modules/ledger/components/ledger-account-delete-dialog"
import { LedgerAccountBalanceCard } from "@/modules/ledger/components/ledger-account-balance-card"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

const routeApi = getRouteApi("/ledger/accounts/$accountId")

export function LedgerAccountPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { accountId } = routeApi.useParams()
  const { page = 1 } = routeApi.useSearch()
  const { data: account } = useLedgerAccount(accountId)
  const { data: categories = [] } = useLedgerCategories()
  const updateAccount = useUpdateLedgerAccount()
  const deleteAccount = useDeleteLedgerAccount()
  const [editing, setEditing] = useState(false)
  const [deleting, setDeleting] = useState(false)

  usePageTitle(account?.name ?? t("nav.ledger"), t("app.title"))

  const changePage = useCallback(
    (next: number, options?: { replace?: boolean }) =>
      void navigate({ to: "/ledger/accounts/$accountId", params: { accountId }, search: ledgerAccountSearch(next), replace: options?.replace }),
    [navigate, accountId],
  )

  function handleDelete() {
    if (!account) return
    deleteAccount.mutate(account.id, {
      onSuccess: () => {
        setDeleting(false)
        toast.success(t("ledger.accountDelete.done", { name: account.name }))
        void navigate({ to: "/ledger" })
      },
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
            <div className="text-sm">{account ? formatDate(account.createdAt) : "-"}</div>
          </div>
        </CardContent>
      </Card>

      {account && <LedgerAccountBalanceCard account={account} />}

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("ledger.transactions")}</h2>
        <LedgerAccountTransactions accountId={accountId} categories={categories} currency={account?.currency} page={page} onPageChange={changePage} />
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
