import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Pencil, Play, Trash2, Wifi } from "lucide-react"
import { useDeleteLedgerEmailAccount, useLedgerEmailScanStatus, useStartLedgerEmailScan, useTestLedgerEmailAccount, useUploadLedgerEmailMessages } from "@/modules/ledger/hooks/use-ledger"
import type { LedgerEmailAccount } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

const MAX_SHOWN_WARNINGS = 5

interface ScanReport {
  ordersNew: number
  warnings?: string[]
  incomplete: boolean
  resumes?: boolean
}

interface LedgerEmailAccountCardProps {
  account: LedgerEmailAccount
  llmOff: boolean
  onEdit: (account: LedgerEmailAccount) => void
}

export function LedgerEmailAccountCard({ account, llmOff, onEdit }: LedgerEmailAccountCardProps) {
  const { t } = useTranslation()
  const deleteAccount = useDeleteLedgerEmailAccount()
  const testAccount = useTestLedgerEmailAccount()
  const startScan = useStartLedgerEmailScan()
  const upload = useUploadLedgerEmailMessages()

  function reportScan(report: ScanReport) {
    toast.success(t("ledger.email.scanSuccess", { count: report.ordersNew }))
    if (report.warnings?.length) {
      const shown = report.warnings.slice(0, MAX_SHOWN_WARNINGS)
      const more = report.warnings.length - shown.length
      toast.warning(t("ledger.email.scanWarnings", { count: report.warnings.length }), {
        description: <span className="whitespace-pre-line">{[...shown, ...(more > 0 ? [t("ledger.email.scanWarningsMore", { count: more })] : [])].join("\n")}</span>,
      })
    }
    if (report.incomplete) {
      toast.warning(t(report.resumes ? "ledger.email.scanPartialResumes" : "ledger.email.scanPartial"))
    }
  }

  const scan = useLedgerEmailScanStatus(account.id, (status) => {
    // An upload reports its own result from the response.
    if (status.trigger === "upload") {
      return
    }
    if (status.state === "failed") {
      toast.error(t("ledger.email.scanFailed", { error: status.error ?? "" }))
      return
    }
    reportScan({ ordersNew: status.ordersNew, warnings: status.warnings, incomplete: status.state === "partial", resumes: status.resumes })
  })
  const scanning = scan?.state === "running" && scan.trigger !== "upload"
  const busy = scan?.state === "running" || startScan.isPending || upload.isPending

  return (
    <div className="rounded-lg border p-4 space-y-3">
      <div>
        <div className="font-medium">{account.name}</div>
        <div className="text-sm text-muted-foreground">{account.username} • {account.imapHost}:{account.imapPort}{account.folder ? ` • ${account.folder}` : ""}</div>
        {account.folder ? null : <div className="text-sm text-amber-600 dark:text-amber-400">{t("ledger.email.folderMissing")}</div>}
        <div className="text-xs text-muted-foreground">{t("ledger.email.lastScan")}: {account.lastScanAt ?? "-"}</div>
        {scanning ? (
          <div className="text-sm" role="status">{t("ledger.email.scanProgress", { scanned: scan.emailsScanned, created: scan.ordersNew })}</div>
        ) : null}
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
          disabled={llmOff || !account.folder || busy}
          onClick={() => startScan.mutate(account.id, { onError: (error) => toast.error(error.message) })}
        >
          <Play className="mr-2 h-4 w-4" />{scanning || startScan.isPending ? t("ledger.email.scanning") : t("ledger.email.scanNow")}
        </Button>
        <label className="inline-flex">
          <Input
            type="file"
            multiple
            accept=".eml,message/rfc822"
            className="hidden"
            disabled={llmOff || busy}
            onChange={(event) => {
              const files = Array.from(event.target.files ?? [])
              if (files.length === 0) {
                return
              }
              upload.mutate({ id: account.id, files }, {
                onSuccess: (result) => reportScan({ ordersNew: result.ordersNew, warnings: result.warnings, incomplete: Boolean(result.partial || result.skipped) }),
                onError: (error) => toast.error(error.message),
              })
              event.target.value = ""
            }}
          />
          <Button type="button" variant="outline" asChild aria-disabled={llmOff || busy} className={llmOff || busy ? "pointer-events-none opacity-50" : undefined}>
            <span>{upload.isPending ? t("ledger.email.scanning") : t("ledger.email.scanUpload")}</span>
          </Button>
        </label>
        <Button type="button" variant="outline" onClick={() => onEdit(account)}><Pencil className="mr-2 h-4 w-4" />{t("common.edit")}</Button>
        <Button type="button" variant="outline" onClick={() => deleteAccount.mutate(account.id, { onSuccess: () => toast.success(t("ledger.email.accountDeleted")), onError: (error) => toast.error(error.message) })}><Trash2 className="mr-2 h-4 w-4" />{t("common.delete")}</Button>
      </div>
    </div>
  )
}
