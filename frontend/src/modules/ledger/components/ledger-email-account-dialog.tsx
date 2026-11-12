import { useState } from "react"
import { useForm, type Resolver } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { useTranslation } from "react-i18next"
import type { LedgerEmailAccount, LedgerEmailAccountInput } from "@/modules/ledger/types"
import { ledgerEmailAccountInputSchema } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import { useCapabilities } from "@/hooks/use-capabilities"

interface LedgerEmailAccountDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  account?: LedgerEmailAccount | null
  onSubmit: (data: LedgerEmailAccountInput) => void
}

export function LedgerEmailAccountDialog({ open, onOpenChange, account, onSubmit }: LedgerEmailAccountDialogProps) {
  const { t } = useTranslation()
  const { llm } = useCapabilities()
  const [today] = useState(() => new Date().toISOString().slice(0, 10))
  const form = useForm<LedgerEmailAccountInput>({
    resolver: standardSchemaResolver(ledgerEmailAccountInputSchema) as Resolver<LedgerEmailAccountInput>,
    defaultValues: {
      name: account?.name ?? "",
      imapHost: account?.imapHost ?? "imap.gmail.com",
      imapPort: account?.imapPort ?? 993,
      username: account?.username ?? "",
      password: "",
      useTls: account?.useTls ?? true,
      folder: account ? account.folder : "INBOX",
      preClassify: account?.preClassify ?? false,
      scanSince: account?.scanSince ?? today,
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{account ? t("ledger.email.editAccount") : t("ledger.email.createAccount")}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form className="space-y-4" onSubmit={form.handleSubmit(onSubmit)}>
            <FormField control={form.control} name="name" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.accountName")}</FormLabel>
                <FormControl><Input {...field} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name="imapHost" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.imapHost")}</FormLabel>
                <FormControl><Input {...field} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name="imapPort" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.imapPort")}</FormLabel>
                <FormControl><Input type="number" value={field.value} onChange={(event) => field.onChange(Number(event.target.value))} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name="username" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.username")}</FormLabel>
                <FormControl><Input {...field} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name="password" render={({ field }) => (
              <FormItem>
                <FormLabel>{account ? t("ledger.email.passwordOptional") : t("ledger.email.password")}</FormLabel>
                <FormControl><Input type="password" {...field} value={field.value ?? ""} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <FormField control={form.control} name="folder" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.folder")}</FormLabel>
                <FormControl><Input placeholder="INBOX" {...field} /></FormControl>
                <FormDescription>{t("ledger.email.folderHint")}</FormDescription>
                <FormMessage />
              </FormItem>
            )} />
            {llm ? (
              <FormField control={form.control} name="preClassify" render={({ field }) => (
                <FormItem>
                  <div className="flex items-center justify-between gap-4">
                    <FormLabel>{t("ledger.email.preClassify")}</FormLabel>
                    <FormControl>
                      <Switch checked={field.value} onCheckedChange={field.onChange} />
                    </FormControl>
                  </div>
                  <FormDescription>{t("ledger.email.preClassifyHint")}</FormDescription>
                </FormItem>
              )} />
            ) : null}
            <FormField control={form.control} name="scanSince" render={({ field }) => (
              <FormItem>
                <FormLabel>{t("ledger.email.scanSince")}</FormLabel>
                <FormControl><Input type="date" {...field} /></FormControl>
                <FormMessage />
              </FormItem>
            )} />
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
              <Button type="submit">{t("common.save")}</Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
