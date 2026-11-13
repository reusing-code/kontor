import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Plus } from "lucide-react"
import { useCreateLedgerEmailAccount, useLedgerEmailAccounts, useUpdateLedgerEmailAccount } from "@/modules/ledger/hooks/use-ledger"
import type { LedgerEmailAccount } from "@/modules/ledger/types"
import { LedgerEmailAccountCard } from "@/modules/ledger/components/ledger-email-account-card"
import { LedgerEmailAccountDialog } from "@/modules/ledger/components/ledger-email-account-dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useCapabilities } from "@/hooks/use-capabilities"
import { LedgerEmailLlmNotice } from "@/modules/ledger/components/ledger-email-llm-notice"

export function LedgerEmailAccountsPanel() {
  const { t } = useTranslation()
  const { llm, isLoading: capabilitiesLoading } = useCapabilities()
  const llmOff = !capabilitiesLoading && !llm
  const { data: accounts = [] } = useLedgerEmailAccounts()
  const createAccount = useCreateLedgerEmailAccount()
  const updateAccount = useUpdateLedgerEmailAccount()
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
        <LedgerEmailLlmNotice />
        {accounts.length === 0 ? <p className="text-sm text-muted-foreground">{t("ledger.email.noAccounts")}</p> : null}
        <p className="text-sm text-muted-foreground">{t("ledger.email.scanHint")}</p>
        <div className="grid gap-4 md:grid-cols-2">
          {accounts.map((account) => (
            <LedgerEmailAccountCard
              key={account.id}
              account={account}
              llmOff={llmOff}
              onEdit={(selected) => { setSelectedAccount(selected); setDialogOpen(true) }}
            />
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
