import { useState } from "react"
import { useForm, useWatch, type Resolver } from "react-hook-form"
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
import { useLedgerEmailFolders } from "@/modules/ledger/hooks/use-ledger"
import { LedgerEmailFolderList } from "@/modules/ledger/components/ledger-email-folder-list"
import { LedgerEmailProcessedAction } from "@/modules/ledger/components/ledger-email-processed-action"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

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
      draftsFolder: account?.draftsFolder ?? "",
      processedAction: account?.processedAction ?? "keep",
      processedFolder: account?.processedFolder ?? "",
      draftsProcessedAction: account?.draftsProcessedAction ?? "keep",
      draftsProcessedFolder: account?.draftsProcessedFolder ?? "",
      preClassify: account?.preClassify ?? false,
      scanSince: account?.scanSince ?? today,
    },
  })
  // an account about to start deleting emails, waiting for the confirmation
  const [confirmDelete, setConfirmDelete] = useState<LedgerEmailAccountInput | null>(null)
  const submit = ({ password, ...values }: LedgerEmailAccountInput) => {
    if (!account && !password) {
      form.setError("password", { message: t("ledger.email.passwordRequired") })
      return
    }
    const draftsAction = values.draftsFolder ? values.draftsProcessedAction : "keep"
    const data: LedgerEmailAccountInput = {
      ...values,
      processedFolder: values.processedAction === "move" ? values.processedFolder : "",
      draftsProcessedAction: draftsAction,
      draftsProcessedFolder: draftsAction === "move" ? values.draftsProcessedFolder : "",
      ...(password ? { password } : {}),
    }
    const startsDeleting =
      (data.processedAction === "delete" && account?.processedAction !== "delete") ||
      (data.draftsProcessedAction === "delete" && account?.draftsProcessedAction !== "delete")
    if (startsDeleting) {
      setConfirmDelete(data)
      return
    }
    onSubmit(data)
  }
  const [draftsFolder, processedAction, processedFolder, draftsProcessedAction, draftsProcessedFolder] = useWatch({
    control: form.control,
    name: ["draftsFolder", "processedAction", "processedFolder", "draftsProcessedAction", "draftsProcessedFolder"],
  })
  const setField = (name: "processedAction" | "processedFolder" | "draftsProcessedAction" | "draftsProcessedFolder") =>
    (value: string) => form.setValue(name, value as never, { shouldDirty: true, shouldValidate: form.formState.isSubmitted })
  const folders = useLedgerEmailFolders()
  const loadFolders = () => {
    const { imapHost, imapPort, username, password, useTls } = form.getValues()
    folders.mutate({
      imapHost,
      imapPort,
      username,
      useTls,
      ...(password ? { password } : {}),
      ...(account ? { accountId: account.id } : {}),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{account ? t("ledger.email.editAccount") : t("ledger.email.createAccount")}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form className="space-y-4" onSubmit={form.handleSubmit(submit)}>
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
                <div className="flex gap-2">
                  <FormControl><Input placeholder="INBOX" {...field} /></FormControl>
                  <Button type="button" variant="outline" disabled={folders.isPending} onClick={loadFolders}>
                    {folders.isPending ? t("common.loading") : t("ledger.email.loadFolders")}
                  </Button>
                </div>
                <LedgerEmailFolderList
                  folders={folders.data}
                  error={folders.error}
                  value={field.value}
                  onSelect={(folder) => form.setValue("folder", folder, { shouldDirty: true, shouldValidate: true })}
                />
                <FormDescription>{t("ledger.email.folderHint")}</FormDescription>
                <FormMessage />
              </FormItem>
            )} />
            {llm ? (
              <FormField control={form.control} name="draftsFolder" render={({ field, fieldState }) => (
                <FormItem>
                  <FormLabel>{t("ledger.email.draftsFolder")}</FormLabel>
                  <FormControl><Input {...field} value={field.value ?? ""} /></FormControl>
                  <LedgerEmailFolderList
                    folders={folders.data}
                    error={null}
                    value={field.value ?? ""}
                    label={t("ledger.email.draftsFolder")}
                    onSelect={(folder) => form.setValue("draftsFolder", folder, { shouldDirty: true, shouldValidate: true })}
                  />
                  <FormDescription>{t("ledger.email.draftsFolderHint")}</FormDescription>
                  {fieldState.error ? <p role="alert" className="text-sm text-destructive">{t(fieldState.error.message ?? "")}</p> : null}
                </FormItem>
              )} />
            ) : null}
            <LedgerEmailProcessedAction
              label={t("ledger.email.processed.orders")}
              action={processedAction}
              onActionChange={setField("processedAction")}
              folder={processedFolder}
              onFolderChange={setField("processedFolder")}
              folders={folders.data}
              folderError={form.formState.errors.processedFolder?.message}
            />
            {llm && draftsFolder?.trim() ? (
              <LedgerEmailProcessedAction
                label={t("ledger.email.processed.drafts")}
                action={draftsProcessedAction}
                onActionChange={setField("draftsProcessedAction")}
                folder={draftsProcessedFolder}
                onFolderChange={setField("draftsProcessedFolder")}
                folders={folders.data}
                folderError={form.formState.errors.draftsProcessedFolder?.message}
              />
            ) : null}
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
      <DeleteConfirmDialog
        open={confirmDelete !== null}
        onOpenChange={(next) => { if (!next) setConfirmDelete(null) }}
        description={t("ledger.email.processed.confirmDelete")}
        actionLabel={t("ledger.email.processed.confirmDeleteAction")}
        onConfirm={() => {
          if (confirmDelete) onSubmit(confirmDelete)
          setConfirmDelete(null)
        }}
      />
    </Dialog>
  )
}
