import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { ledgerAccountUpdateInputSchema, type LedgerAccount, type LedgerAccountUpdateInput } from "@/modules/ledger/types"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"

interface LedgerAccountEditDialogProps {
  account: LedgerAccount
  open: boolean
  onOpenChange: (open: boolean) => void
  pending?: boolean
  onSubmit: (data: LedgerAccountUpdateInput) => void
}

// LedgerAccountEditDialog edits name and bank; IBAN and currency are fixed.
export function LedgerAccountEditDialog({ account, open, onOpenChange, pending, onSubmit }: LedgerAccountEditDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <EditForm account={account} onOpenChange={onOpenChange} pending={pending} onSubmit={onSubmit} /> : null}
    </Dialog>
  )
}

function EditForm({ account, onOpenChange, pending, onSubmit }: Omit<LedgerAccountEditDialogProps, "open">) {
  const { t } = useTranslation()
  const form = useForm<LedgerAccountUpdateInput>({
    resolver: standardSchemaResolver(ledgerAccountUpdateInputSchema) as Resolver<LedgerAccountUpdateInput>,
    defaultValues: { name: account.name, bank: account.bank },
  })
  const fields = [
    { name: "name", label: t("ledger.accountName") },
    { name: "bank", label: t("ledger.bank") },
  ] as const

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t("ledger.accountEdit.title")}</DialogTitle>
        <DialogDescription>{t("ledger.accountEdit.description")}</DialogDescription>
      </DialogHeader>
      <Form {...form}>
        <form className="space-y-4" onSubmit={form.handleSubmit(onSubmit)}>
          {fields.map(({ name, label }) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{label}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>{t("common.save")}</Button>
          </DialogFooter>
        </form>
      </Form>
    </DialogContent>
  )
}
