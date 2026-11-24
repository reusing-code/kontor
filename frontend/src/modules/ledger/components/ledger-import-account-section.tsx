import { useTranslation } from "react-i18next"
import type { LedgerImportFlow } from "@/modules/ledger/hooks/use-ledger-import-flow"
import type { LedgerAccount } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
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

export function LedgerImportAccountSection({ flow, accounts }: { flow: LedgerImportFlow; accounts: LedgerAccount[] }) {
  const { t } = useTranslation()
  const { mode, setMode, newAccountForm } = flow

  return (
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
            <Select value={flow.selectedAccountId} onValueChange={flow.setSelectedAccountId}>
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
  )
}
