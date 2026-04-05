import { useMemo, useState } from "react"
import { useTranslation } from "react-i18next"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { useForm } from "react-hook-form"
import { toast } from "sonner"
import { Upload } from "lucide-react"
import { useLedgerCommitImport, useLedgerPreviewImport } from "@/hooks/use-ledger"
import { defaultLedgerAccountInput } from "@/lib/ledger-repository"
import { formatAmountMinor, formatLedgerDate, formatSourceType } from "@/lib/ledger-utils"
import { ledgerAccountInputSchema, type LedgerAccount, type LedgerAccountInput, type LedgerPreviewResult, type LedgerSourceType } from "@/types/ledger"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Input } from "@/components/ui/input"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

interface LedgerImportPanelProps {
  accounts: LedgerAccount[]
}

export function LedgerImportPanel({ accounts }: LedgerImportPanelProps) {
  const { t, i18n } = useTranslation()
  const previewMutation = useLedgerPreviewImport()
  const commitMutation = useLedgerCommitImport()
  const [file, setFile] = useState<File | null>(null)
  const [sourceType, setSourceType] = useState<LedgerSourceType>("dkb.csv")
  const [selectedAccountId, setSelectedAccountId] = useState<string>("")
  const [preview, setPreview] = useState<LedgerPreviewResult | null>(null)
  const [mode, setMode] = useState<"existing" | "new">("existing")

  const newAccountForm = useForm<LedgerAccountInput>({
    resolver: standardSchemaResolver(ledgerAccountInputSchema),
    defaultValues: defaultLedgerAccountInput(),
  })

  const unresolvedAccount = preview && !preview.accountId
  const canPreview = !!file
  const canCommit = !!preview && (mode === "new" || selectedAccountId || preview.accountId)

  const accountNameById = useMemo(
    () => new Map(accounts.map((account) => [account.id, account.name])),
    [accounts],
  )

  function resetFlow() {
    setFile(null)
    setSelectedAccountId("")
    setPreview(null)
    setMode("existing")
    newAccountForm.reset(defaultLedgerAccountInput())
    previewMutation.reset()
    commitMutation.reset()
  }

  function handlePreview() {
    if (!file) return
    previewMutation.mutate(
      {
        file,
        sourceType,
        accountId: selectedAccountId || undefined,
      },
      {
        onSuccess: (result) => {
          setPreview(result)
          setSelectedAccountId(result.accountId ?? selectedAccountId)
          newAccountForm.reset(defaultLedgerAccountInput(result.iban, result.bankName))
          if (!result.accountId) {
            setMode(accounts.length > 0 ? "existing" : "new")
          }
          toast.success(t("ledger.previewReady"))
        },
        onError: (error) => {
          toast.error(error.message)
        },
      },
    )
  }

  function handleCommit() {
    if (!preview) return

    if (mode === "new") {
      newAccountForm.handleSubmit((data) => {
        commitMutation.mutate(
          {
            previewId: preview.previewId,
            data: { newAccount: data },
          },
          {
            onSuccess: (result) => {
              toast.success(t("ledger.importCommitted", { count: result.importedRows }))
              resetFlow()
            },
            onError: (error) => {
              toast.error(error.message)
            },
          },
        )
      })()
      return
    }

    commitMutation.mutate(
      {
        previewId: preview.previewId,
        data: { accountId: selectedAccountId || preview.accountId },
      },
      {
        onSuccess: (result) => {
          toast.success(t("ledger.importCommitted", { count: result.importedRows }))
          resetFlow()
        },
        onError: (error) => {
          toast.error(error.message)
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.importTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-4 md:grid-cols-3">
          <div className="space-y-2">
            <label className="text-sm font-medium">{t("ledger.sourceType")}</label>
            <Select value={sourceType} onValueChange={(value) => setSourceType(value as LedgerSourceType)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="dkb.csv">DKB CSV</SelectItem>
                <SelectItem value="comdirect.csv">comdirect CSV</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2 md:col-span-2">
            <label className="text-sm font-medium">{t("ledger.file")}</label>
            <div className="flex gap-2">
              <Input type="file" accept=".csv,text/csv" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
              <Button type="button" onClick={handlePreview} disabled={!canPreview || previewMutation.isPending}>
                <Upload className="mr-2 h-4 w-4" />
                {previewMutation.isPending ? t("ledger.previewing") : t("ledger.preview")}
              </Button>
            </div>
          </div>
        </div>

        {sourceType === "comdirect.csv" && accounts.length > 0 && !preview && (
          <div className="space-y-2">
            <label className="text-sm font-medium">{t("ledger.account")}</label>
            <Select value={selectedAccountId} onValueChange={setSelectedAccountId}>
              <SelectTrigger className="w-full md:max-w-md">
                <SelectValue placeholder={t("ledger.selectAccountOptional")} />
              </SelectTrigger>
              <SelectContent>
                {accounts.map((account) => (
                  <SelectItem key={account.id} value={account.id}>
                    {account.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {preview && (
          <div className="space-y-4 rounded-md border p-4">
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary">{formatSourceType(preview.sourceType)}</Badge>
              {preview.bankName && <Badge variant="outline">{preview.bankName}</Badge>}
              {preview.iban && <Badge variant="outline">{preview.iban}</Badge>}
              {(preview.accountId || selectedAccountId) && (
                <Badge variant="outline">{accountNameById.get(selectedAccountId || preview.accountId || "") ?? t("ledger.accountResolved")}</Badge>
              )}
            </div>

            <div className="grid gap-4 sm:grid-cols-3">
              <div>
                <div className="text-sm text-muted-foreground">{t("ledger.totalRows")}</div>
                <div className="text-2xl font-semibold">{preview.totalRows}</div>
              </div>
              <div>
                <div className="text-sm text-muted-foreground">{t("ledger.newRows")}</div>
                <div className="text-2xl font-semibold">{preview.newRows}</div>
              </div>
              <div>
                <div className="text-sm text-muted-foreground">{t("ledger.duplicateRows")}</div>
                <div className="text-2xl font-semibold">{preview.duplicateRows}</div>
              </div>
            </div>

            {preview.warnings && preview.warnings.length > 0 && (
              <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 p-3 text-sm">
                <div className="font-medium">{t("ledger.warnings")}</div>
                <ul className="mt-2 space-y-1 text-muted-foreground">
                  {preview.warnings.map((warning) => (
                    <li key={warning}>{warning}</li>
                  ))}
                </ul>
              </div>
            )}

            {unresolvedAccount && (
              <div className="space-y-4 rounded-md border p-4">
                <div className="text-sm font-medium">{t("ledger.resolveAccount")}</div>

                {accounts.length > 0 && (
                  <div className="space-y-3">
                    <div className="flex gap-2">
                      <Button type="button" variant={mode === "existing" ? "default" : "outline"} onClick={() => setMode("existing")}>
                        {t("ledger.useExistingAccount")}
                      </Button>
                      <Button type="button" variant={mode === "new" ? "default" : "outline"} onClick={() => setMode("new")}>
                        {t("ledger.createAccount")}
                      </Button>
                    </div>

                    {mode === "existing" && (
                      <Select value={selectedAccountId} onValueChange={setSelectedAccountId}>
                        <SelectTrigger className="w-full md:max-w-md">
                          <SelectValue placeholder={t("ledger.selectAccount")} />
                        </SelectTrigger>
                        <SelectContent>
                          {accounts.map((account) => (
                            <SelectItem key={account.id} value={account.id}>
                              {account.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  </div>
                )}

                {(mode === "new" || accounts.length === 0) && (
                  <Form {...newAccountForm}>
                    <form className="grid gap-4 md:grid-cols-2" onSubmit={(e) => e.preventDefault()}>
                      <FormField
                        control={newAccountForm.control}
                        name="name"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t("ledger.accountName")}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={newAccountForm.control}
                        name="bank"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t("ledger.bank")}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={newAccountForm.control}
                        name="iban"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t("ledger.iban")}</FormLabel>
                            <FormControl>
                              <Input {...field} value={field.value ?? ""} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={newAccountForm.control}
                        name="currency"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t("ledger.currency")}</FormLabel>
                            <FormControl>
                              <Input {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </form>
                  </Form>
                )}
              </div>
            )}

            <div className="overflow-x-auto rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("ledger.date")}</TableHead>
                    <TableHead>{t("ledger.valueDate")}</TableHead>
                    <TableHead>{t("ledger.amount")}</TableHead>
                    <TableHead>{t("ledger.counterparty")}</TableHead>
                    <TableHead>{t("ledger.purpose")}</TableHead>
                    <TableHead>{t("ledger.status")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {preview.transactions.map((txn) => (
                    <TableRow key={txn.fingerprint}>
                      <TableCell>{formatLedgerDate(txn.row.bookingDate)}</TableCell>
                      <TableCell>{formatLedgerDate(txn.row.valueDate)}</TableCell>
                      <TableCell className={txn.row.amountMinor < 0 ? "text-destructive" : "text-emerald-600"}>
                        {formatAmountMinor(txn.row.amountMinor, txn.row.currency, i18n.language)}
                      </TableCell>
                      <TableCell>{txn.row.counterpartyName || "-"}</TableCell>
                      <TableCell className="max-w-[24rem] truncate">{txn.row.purpose || "-"}</TableCell>
                      <TableCell>
                        <Badge variant={txn.isDuplicate ? "secondary" : "default"}>
                          {txn.isDuplicate ? t("ledger.duplicate") : t("ledger.new")}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <div className="flex flex-wrap justify-end gap-2">
              <Button type="button" variant="outline" onClick={resetFlow}>
                {t("common.cancel")}
              </Button>
              <Button type="button" onClick={handleCommit} disabled={!canCommit || commitMutation.isPending}>
                {commitMutation.isPending ? t("ledger.committing") : t("ledger.commit")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
