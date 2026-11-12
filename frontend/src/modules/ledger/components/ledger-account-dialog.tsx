import { useTranslation } from "react-i18next"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { ledgerAccountInputSchema, type LedgerAccountInput } from "@/modules/ledger/types"
import { defaultLedgerAccountInput } from "@/modules/ledger/lib/ledger-repository"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Button } from "@/components/ui/button"

interface LedgerAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  // Prefills the form. Pass a stable object.
  initialValues?: Partial<LedgerAccountInput>
  onSubmit: (data: LedgerAccountInput) => void
}

export function LedgerAccountDialog({ open, onOpenChange, initialValues, onSubmit }: LedgerAccountDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {open ? <LedgerAccountDialogForm initialValues={initialValues} onOpenChange={onOpenChange} onSubmit={onSubmit} /> : null}
    </Dialog>
  )
}

function LedgerAccountDialogForm({
  initialValues,
  onOpenChange,
  onSubmit,
}: Omit<LedgerAccountDialogProps, "open">) {
  const { t } = useTranslation()
  const form = useForm<LedgerAccountInput>({
    resolver: standardSchemaResolver(ledgerAccountInputSchema) as Resolver<LedgerAccountInput>,
    defaultValues: { ...defaultLedgerAccountInput(), ...initialValues },
  })

  const fields = [
    { name: "name", label: t("ledger.accountName"), placeholder: t("ledger.accountDialog.namePlaceholder") },
    { name: "bank", label: t("ledger.bank"), placeholder: t("ledger.accountDialog.bankPlaceholder") },
    { name: "iban", label: t("ledger.iban"), placeholder: "DE00 0000 0000 0000 0000 00" },
    { name: "currency", label: t("ledger.accountDialog.currency"), placeholder: "EUR" },
  ] as const

  return (
    <DialogContent>
      <DialogHeader>
        <DialogTitle>{t("ledger.accountDialog.create")}</DialogTitle>
      </DialogHeader>
      <Form {...form}>
        <form className="space-y-4" onSubmit={form.handleSubmit((data) => onSubmit({ ...data, iban: data.iban?.trim() || undefined }))}>
          {fields.map(({ name, label, placeholder }) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{label}</FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ""} placeholder={placeholder} />
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
            <Button type="submit">{t("common.save")}</Button>
          </DialogFooter>
        </form>
      </Form>
    </DialogContent>
  )
}
