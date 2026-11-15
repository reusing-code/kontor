import { useTranslation } from "react-i18next"
import { cn } from "@/lib/utils"
import { folderErrorKey, visibleFolders } from "@/modules/ledger/lib/email-folders"

interface LedgerEmailFolderListProps {
  folders: string[] | undefined
  error: unknown
  value: string
  onSelect: (folder: string) => void
}

export function LedgerEmailFolderList({ folders, error, value, onSelect }: LedgerEmailFolderListProps) {
  const { t } = useTranslation()
  if (error) {
    return <p role="alert" className="text-sm text-destructive">{t(folderErrorKey(error))}</p>
  }
  if (!folders) return null
  if (folders.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("ledger.email.foldersNone")}</p>
  }
  const visible = visibleFolders(folders, value)
  if (visible.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("ledger.email.foldersNoMatch")}</p>
  }
  return (
    <ul className="max-h-40 space-y-1 overflow-y-auto" aria-label={t("ledger.email.folder")}>
      {visible.map((folder) => (
        <li key={folder}>
          <button
            type="button"
            onClick={() => onSelect(folder)}
            aria-pressed={folder === value.trim()}
            className={cn(
              "w-full rounded-md border px-3 py-1.5 text-left text-sm hover:bg-accent",
              folder === value.trim() && "border-primary bg-accent",
            )}
          >
            {folder}
          </button>
        </li>
      ))}
    </ul>
  )
}
