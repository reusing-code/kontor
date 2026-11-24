import { useMemo, useState } from "react"
import { formatDate, formatDateTime } from "@/lib/format"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Link2, Plus, RefreshCw, Trash2 } from "lucide-react"
import {
  useBankConnections,
  useDisconnectBank,
  useMapBankAccount,
  useSyncBankConnection,
  useUnmapBankAccount,
} from "@/modules/ledger/hooks/use-bank-connections"
import { useStartBankAuthorization } from "@/modules/ledger/hooks/use-bank-authorization"
import { bankErrorMessage } from "@/modules/ledger/lib/bank-errors"
import { useCreateLedgerAccount, useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger-accounts"
import type { BankAccount, BankConnection, LedgerAccount, LedgerAccountInput } from "@/modules/ledger/types"
import { BankConnectDialog } from "@/modules/ledger/components/bank-connect-dialog"
import { BankConsentBanner } from "@/modules/ledger/components/bank-consent-banner"
import { LedgerAccountDialog } from "@/modules/ledger/components/ledger-account-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { cn } from "@/lib/utils"

const knownStatusCodes = [
  "ok",
  "unmapped",
  "consent_expired",
  "access_denied",
  "account_not_accessible",
  "rate_limited",
  "bank_unavailable",
  "app_error",
  "timeout",
  "error",
]

function normalizeIban(value?: string): string {
  return (value ?? "").replace(/\s+/g, "").toUpperCase()
}

interface PendingCreate {
  connectionId: string
  account: BankAccount
  bank: string
}

export function BankConnectionsPanel() {
  const { t } = useTranslation()
  const { data: connections = [], isLoading, error } = useBankConnections()
  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const { start, isPending: authorizing } = useStartBankAuthorization()
  const disconnect = useDisconnectBank()
  const mapAccount = useMapBankAccount()
  const unmapAccount = useUnmapBankAccount()
  const createAccount = useCreateLedgerAccount()
  const [connectOpen, setConnectOpen] = useState(false)
  const [toDisconnect, setToDisconnect] = useState<BankConnection | null>(null)
  const [pendingCreate, setPendingCreate] = useState<PendingCreate | null>(null)

  const mappedLedgerIds = useMemo(
    () => new Set(connections.flatMap((c) => c.accounts.flatMap((a) => (a.ledgerAccountId ? [a.ledgerAccountId] : [])))),
    [connections],
  )

  const createInitialValues = useMemo<Partial<LedgerAccountInput> | undefined>(
    () =>
      pendingCreate
        ? {
            name: pendingCreate.account.name || pendingCreate.bank,
            bank: pendingCreate.bank,
            iban: pendingCreate.account.iban,
            currency: pendingCreate.account.currency,
          }
        : undefined,
    [pendingCreate],
  )

  function handleDisconnect() {
    if (!toDisconnect) return
    const target = toDisconnect
    setToDisconnect(null)
    disconnect.mutate(target.id, {
      onSuccess: ({ remoteClosed }) => {
        toast.success(t("ledger.bankImport.disconnected", { bank: target.aspspName }))
        if (!remoteClosed) toast.warning(t("ledger.bankImport.remoteNotClosed"))
      },
    })
  }

  async function handleCreate(input: LedgerAccountInput) {
    if (!pendingCreate) return
    const { connectionId, account } = pendingCreate
    const created = await createAccount.mutateAsync(input)
    setPendingCreate(null)
    mapAccount.mutate({ id: connectionId, uid: account.uid, ledgerAccountId: created.id })
  }

  return (
    <div className="space-y-4">
      <BankConsentBanner />

      <div className="flex justify-end">
        <Button type="button" onClick={() => setConnectOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          {t("ledger.bankImport.connect.button")}
        </Button>
      </div>

      {error ? (
        <p role="alert" className="text-sm text-destructive">{bankErrorMessage(t, error)}</p>
      ) : isLoading ? (
        <p className="text-sm text-muted-foreground">{t("common.loading")}</p>
      ) : connections.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("ledger.bankImport.empty")}</p>
      ) : (
        connections.map((connection) => (
          <BankConnectionCard
            key={connection.id}
            connection={connection}
            ledgerAccounts={ledgerAccounts}
            mappedLedgerIds={mappedLedgerIds}
            reauthorizing={authorizing}
            onReauthorize={() => start({ aspspName: connection.aspspName, aspspCountry: connection.aspspCountry })}
            onDisconnect={() => setToDisconnect(connection)}
            onMap={(uid, ledgerAccountId) => mapAccount.mutate({ id: connection.id, uid, ledgerAccountId })}
            onUnmap={(uid) =>
              unmapAccount.mutate(
                { id: connection.id, uid },
                { onSuccess: () => toast.success(t("ledger.bankImport.unmapped")) },
              )
            }
            onCreate={(account) => setPendingCreate({ connectionId: connection.id, account, bank: connection.aspspName })}
          />
        ))
      )}

      <BankConnectDialog
        open={connectOpen}
        onOpenChange={setConnectOpen}
        pending={authorizing}
        onConfirm={(bank) => start({ aspspName: bank.name, aspspCountry: bank.country })}
      />
      <LedgerAccountDialog
        open={pendingCreate !== null}
        onOpenChange={(open) => !open && setPendingCreate(null)}
        initialValues={createInitialValues}
        onSubmit={handleCreate}
      />
      <DeleteConfirmDialog
        open={toDisconnect !== null}
        onOpenChange={(open) => !open && setToDisconnect(null)}
        description={t("ledger.bankImport.disconnectConfirm", { bank: toDisconnect?.aspspName ?? "" })}
        actionLabel={t("ledger.bankImport.disconnect")}
        onConfirm={handleDisconnect}
      />
    </div>
  )
}

interface BankConnectionCardProps {
  connection: BankConnection
  ledgerAccounts: LedgerAccount[]
  mappedLedgerIds: Set<string>
  reauthorizing: boolean
  onReauthorize: () => void
  onDisconnect: () => void
  onMap: (uid: string, ledgerAccountId: string) => void
  onUnmap: (uid: string) => void
  onCreate: (account: BankAccount) => void
}

function BankConnectionCard({
  connection,
  ledgerAccounts,
  mappedLedgerIds,
  reauthorizing,
  onReauthorize,
  onDisconnect,
  onMap,
  onUnmap,
  onCreate,
}: BankConnectionCardProps) {
  const { t } = useTranslation()
  const sync = useSyncBankConnection()

  function handleSync() {
    sync.mutate(connection.id, {
      onSuccess: (summary) => toast.success(t("ledger.bankImport.syncDone", { imported: summary.imported, duplicates: summary.duplicates })),
    })
  }

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2">
            {connection.aspspName}
            <span className="text-sm font-normal text-muted-foreground">{connection.aspspCountry}</span>
            <Badge
              variant={connection.status === "expired" ? "destructive" : "secondary"}
              className={cn(connection.status === "expiring" && "bg-amber-500/20 text-amber-700 dark:text-amber-400")}
            >
              {t(`ledger.bankImport.connectionStatus.${connection.status}`)}
            </Badge>
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {t("ledger.bankImport.validUntil", { date: formatDate(connection.expiresAt) })}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" disabled={sync.isPending || connection.status === "expired"} onClick={handleSync}>
            <RefreshCw className={cn("mr-2 h-4 w-4", sync.isPending && "animate-spin")} />
            {sync.isPending ? t("ledger.bankImport.syncing") : t("ledger.bankImport.syncNow")}
          </Button>
          <Button type="button" variant="outline" disabled={reauthorizing} onClick={onReauthorize}>
            <Link2 className="mr-2 h-4 w-4" />
            {t("ledger.bankImport.reauthorize")}
          </Button>
          <Button type="button" variant="outline" onClick={onDisconnect}>
            <Trash2 className="mr-2 h-4 w-4" />
            {t("ledger.bankImport.disconnect")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {connection.accounts.map((account) => (
          <BankAccountRow
            key={account.uid}
            account={account}
            balancesRequested={connection.balancesRequested}
            ledgerAccounts={ledgerAccounts}
            mappedLedgerIds={mappedLedgerIds}
            onMap={(ledgerAccountId) => onMap(account.uid, ledgerAccountId)}
            onUnmap={() => onUnmap(account.uid)}
            onCreate={() => onCreate(account)}
          />
        ))}
      </CardContent>
    </Card>
  )
}

interface BankAccountRowProps {
  account: BankAccount
  balancesRequested: boolean
  ledgerAccounts: LedgerAccount[]
  mappedLedgerIds: Set<string>
  onMap: (ledgerAccountId: string) => void
  onUnmap: () => void
  onCreate: () => void
}

function BankAccountRow({ account, balancesRequested, ledgerAccounts, mappedLedgerIds, onMap, onUnmap, onCreate }: BankAccountRowProps) {
  const { t } = useTranslation()
  const [changing, setChanging] = useState(false)
  const mappedTo = ledgerAccounts.find((l) => l.id === account.ledgerAccountId)
  const showMapping = !account.mapped || changing
  const candidates = ledgerAccounts.filter(
    (l) =>
      !mappedLedgerIds.has(l.id) &&
      l.currency.toUpperCase() === account.currency.toUpperCase() &&
      (!l.iban || normalizeIban(l.iban) === normalizeIban(account.iban)),
  )
  const statusKey = knownStatusCodes.includes(account.lastSyncStatus) ? account.lastSyncStatus : "error"

  return (
    <div className="space-y-2 rounded-md border p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="font-medium">{account.name || account.iban}</div>
          <div className="text-xs text-muted-foreground">
            {account.iban} · {account.currency}
          </div>
        </div>
        {account.mapped && (
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{t("ledger.bankImport.mappedTo", { name: mappedTo?.name ?? t("ledger.bankImport.ledgerAccount") })}</Badge>
            <Button type="button" variant="ghost" size="sm" onClick={() => setChanging((c) => !c)} aria-expanded={changing}>
              {changing ? t("common.cancel") : t("ledger.bankImport.changeMapping")}
            </Button>
            <Button type="button" variant="ghost" size="sm" onClick={onUnmap}>
              {t("ledger.bankImport.unmap")}
            </Button>
          </div>
        )}
      </div>

      {changing && <p className="text-xs text-muted-foreground">{t("ledger.bankImport.changeMappingHint")}</p>}

      {showMapping && (
        <div className="flex flex-wrap items-center gap-2">
          <Select
            onValueChange={(id) => {
              setChanging(false)
              onMap(id)
            }}
          >
            <SelectTrigger className="w-full md:w-72" aria-label={t("ledger.bankImport.mapTo")}>
              <SelectValue placeholder={candidates.length ? t("ledger.bankImport.mapTo") : t("ledger.bankImport.noCandidates")} />
            </SelectTrigger>
            <SelectContent>
              {candidates.map((l) => (
                <SelectItem key={l.id} value={l.id}>
                  {l.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button type="button" variant="outline" onClick={onCreate}>
            {t("ledger.bankImport.createFromThis")}
          </Button>
        </div>
      )}

      <div className="text-xs text-muted-foreground">
        {account.lastSyncAttemptAt
          ? t("ledger.bankImport.lastSync", {
              time: formatDateTime(account.lastSyncAttemptAt),
              status: t(`ledger.bankImport.status.${statusKey}`),
            })
          : t("ledger.bankImport.neverSynced")}
        {account.lastSyncStatus === "ok" && ` · ${t("ledger.bankImport.lastCounts", { imported: account.lastImported, duplicates: account.lastDuplicates })}`}
      </div>
      {account.mapped && account.lastBalanceStatus === "unavailable" && (
        <p className="text-xs text-yellow-700 dark:text-yellow-500">
          {balancesRequested ? t("ledger.balance.bankUnavailable") : t("ledger.balance.bankUnavailableOldConsent")}
        </p>
      )}
      {account.mapped && account.lastBalanceStatus === "error" && <p className="text-xs text-muted-foreground">{t("ledger.balance.bankError")}</p>}
    </div>
  )
}
