import { useTranslation } from "react-i18next"
import { Upload } from "lucide-react"
import type { LedgerImportFlow } from "@/modules/ledger/hooks/use-ledger-import-flow"
import { formatSourceType } from "@/modules/ledger/lib/ledger-utils"
import type { LedgerAccount, LedgerFileSourceType } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

export function LedgerImportUpload({ flow, accounts }: { flow: LedgerImportFlow; accounts: LedgerAccount[] }) {
  const { t } = useTranslation()

  return (
    <>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="space-y-2">
          <label className="text-sm font-medium">{t("ledger.sourceType")}</label>
          <Select value={flow.sourceType} onValueChange={(value) => flow.setSourceType(value as LedgerFileSourceType)}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="dkb.csv">{formatSourceType("dkb.csv")}</SelectItem>
              <SelectItem value="comdirect.csv">{formatSourceType("comdirect.csv")}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-2 md:col-span-2">
          <label className="text-sm font-medium">{t("ledger.file")}</label>
          <div className="flex gap-2">
            <Input type="file" accept=".csv,text/csv" onChange={(e) => flow.setFile(e.target.files?.[0] ?? null)} />
            <Button type="button" onClick={flow.handlePreview} disabled={!flow.canPreview || flow.isPreviewing}>
              <Upload className="mr-2 h-4 w-4" />
              {flow.isPreviewing ? t("ledger.previewing") : t("ledger.preview")}
            </Button>
          </div>
        </div>
      </div>

      {flow.sourceType === "comdirect.csv" && accounts.length > 0 && !flow.preview && (
        <div className="space-y-2">
          <label className="text-sm font-medium">{t("ledger.account")}</label>
          <Select value={flow.selectedAccountId} onValueChange={flow.setSelectedAccountId}>
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
    </>
  )
}
