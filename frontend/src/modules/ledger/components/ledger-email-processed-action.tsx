import { useTranslation } from "react-i18next"
import { RadioGroup } from "radix-ui"
import { Input } from "@/components/ui/input"
import { emailProcessedActions, type EmailProcessedAction } from "@/modules/ledger/types"
import { LedgerEmailFolderList } from "@/modules/ledger/components/ledger-email-folder-list"

interface LedgerEmailProcessedActionProps {
  label: string
  action: EmailProcessedAction
  onActionChange: (action: EmailProcessedAction) => void
  folder: string
  onFolderChange: (folder: string) => void
  // loaded folder list of the account dialog, reused for the move target
  folders: string[] | undefined
  // translation key of the target folder's validation error
  folderError?: string
}

// What happens in the mailbox to the processed emails of one scanned folder.
export function LedgerEmailProcessedAction({
  label, action, onActionChange, folder, onFolderChange, folders, folderError,
}: LedgerEmailProcessedActionProps) {
  const { t } = useTranslation()
  const targetLabel = t("ledger.email.processed.target", { label })
  return (
    <div className="space-y-2">
      <div className="text-sm font-medium">{label}</div>
      <RadioGroup.Root
        value={action}
        onValueChange={(value) => onActionChange(value as EmailProcessedAction)}
        orientation="horizontal"
        aria-label={label}
        className="flex gap-2"
      >
        {emailProcessedActions.map((option) => (
          <RadioGroup.Item
            key={option}
            value={option}
            className="rounded-md border px-3 py-1.5 text-sm hover:bg-accent data-[state=checked]:border-primary data-[state=checked]:bg-accent"
          >
            {t(`ledger.email.processed.${option}`)}
          </RadioGroup.Item>
        ))}
      </RadioGroup.Root>
      {action === "move" ? (
        <>
          <Input
            aria-label={targetLabel}
            placeholder={t("ledger.email.processed.targetPlaceholder")}
            value={folder}
            onChange={(event) => onFolderChange(event.target.value)}
          />
          <LedgerEmailFolderList folders={folders} error={null} value={folder} label={targetLabel} onSelect={onFolderChange} />
        </>
      ) : null}
      {folderError ? <p role="alert" className="text-sm text-destructive">{t(folderError)}</p> : null}
      <p className="text-sm text-muted-foreground">{t(`ledger.email.processed.${action}Hint`)}</p>
    </div>
  )
}
