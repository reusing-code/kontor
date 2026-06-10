import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Pencil, Play, Plus, Trash2, Wifi } from "lucide-react"
import { useCreateLedgerEmailAccount, useDeleteLedgerEmailAccount, useLedgerEmailAccounts, useScanLedgerEmailAccount, useTestLedgerEmailAccount, useUpdateLedgerEmailAccount } from "@/hooks/use-ledger"
import type { LedgerEmailAccount } from "@/types/ledger"
import { LedgerEmailAccountDialog } from "@/components/ledger-email-account-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

export function LedgerEmailAccountsPanel() {
  const { t } = useTranslation()
  const { data: accounts = [] } = useLedgerEmailAccounts()
  const createAccount = useCreateLedgerEmailAccount()
  const updateAccount = useUpdateLedgerEmailAccount()
  const deleteAccount = useDeleteLedgerEmailAccount()
  const scanAccount = useScanLedgerEmailAccount()
  const testAccount = useTestLedgerEmailAccount()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedAccount, setSelectedAccount] = useState<LedgerEmailAccount | null>(null)

  function openCreate() {
    setSelectedAccount(null)
    setDialogOpen(true)
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-4">
        <CardTitle>{t("ledger.email.accounts")}</CardTitle>
        <Button type="button" onClick={openCreate}><Plus className="mr-2 h-4 w-4" />{t("ledger.email.addAccount")}</Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {accounts.length === 0 ? <p className="text-sm text-muted-foreground">{t("ledger.email.noAccounts")}</p> : null}
        <p className="text-sm text-muted-foreground">{t("ledger.email.scanHint")}</p>
        <div className="grid gap-4 md:grid-cols-2">
          {accounts.map((account) => (
            <div key={account.id} className="rounded-lg border p-4 space-y-3">
              <div>
                <div className="font-medium">{account.name}</div>
                <div className="text-sm text-muted-foreground">{account.username} • {account.imapHost}:{account.imapPort}</div>
                <div className="text-xs text-muted-foreground">{t("ledger.email.lastScan")}: {account.lastScanAt ?? "-"}</div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => testAccount.mutate(account.id, {
                    onSuccess: () => toast.success(t("ledger.email.testSuccess")),
                    onError: (error) => toast.error(error.message),
                  })}
                >
                  <Wifi className="mr-2 h-4 w-4" />{t("ledger.email.testConnection")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => scanAccount.mutate({ id: account.id, files: [] }, {
                    onSuccess: (result) => toast.success(t("ledger.email.scanSuccess", { count: result.ordersNew })),
                    onError: (error) => toast.error(error.message),
                  })}
                >
                  <Play className="mr-2 h-4 w-4" />{t("ledger.email.scanNow")}
                </Button>
                <label className="inline-flex">
                  <Input
                    type="file"
                    multiple
                    accept=".eml,message/rfc822"
                    className="hidden"
                    onChange={(event) => {
                      const files = Array.from(event.target.files ?? [])
                      if (files.length === 0) {
                        return
                      }
                      scanAccount.mutate({ id: account.id, files }, {
                        onSuccess: (result) => toast.success(t("ledger.email.scanSuccess", { count: result.ordersNew })),
                        onError: (error) => toast.error(error.message),
                      })
                      event.target.value = ""
                    }}
                  />
                  <Button type="button" variant="outline" asChild>
                    <span>{t("ledger.email.scanUpload")}</span>
                  </Button>
                </label>
                <Button type="button" variant="outline" onClick={() => { setSelectedAccount(account); setDialogOpen(true) }}><Pencil className="mr-2 h-4 w-4" />{t("common.edit")}</Button>
                <Button type="button" variant="outline" onClick={() => deleteAccount.mutate(account.id, { onSuccess: () => toast.success(t("ledger.email.accountDeleted")), onError: (error) => toast.error(error.message) })}><Trash2 className="mr-2 h-4 w-4" />{t("common.delete")}</Button>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
      <LedgerEmailAccountDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        account={selectedAccount}
        onSubmit={(data) => {
          if (selectedAccount) {
            updateAccount.mutate({ id: selectedAccount.id, data }, {
              onSuccess: () => {
                toast.success(t("ledger.email.accountUpdated"))
                setDialogOpen(false)
              },
              onError: (error) => toast.error(error.message),
            })
            return
          }
          createAccount.mutate(data, {
            onSuccess: () => {
              toast.success(t("ledger.email.accountCreated"))
              setDialogOpen(false)
            },
            onError: (error) => toast.error(error.message),
          })
        }}
      />
    </Card>
  )
}
