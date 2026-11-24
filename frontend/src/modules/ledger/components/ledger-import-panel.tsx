import { useMemo } from "react"
import { useTranslation } from "react-i18next"
import { useLedgerImportFlow } from "@/modules/ledger/hooks/use-ledger-import-flow"
import { LedgerImportAccountSection } from "@/modules/ledger/components/ledger-import-account-section"
import { LedgerImportPreviewSummary, LedgerImportPreviewTable } from "@/modules/ledger/components/ledger-import-preview"
import { LedgerImportUpload } from "@/modules/ledger/components/ledger-import-upload"
import type { LedgerAccount } from "@/modules/ledger/types"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface LedgerImportPanelProps {
  accounts: LedgerAccount[]
}

export function LedgerImportPanel({ accounts }: LedgerImportPanelProps) {
  const { t } = useTranslation()
  const flow = useLedgerImportFlow(accounts)
  const { preview } = flow

  const accountNameById = useMemo(
    () => new Map(accounts.map((account) => [account.id, account.name])),
    [accounts],
  )
  const resolvedAccountId = flow.selectedAccountId || preview?.accountId || ""

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("ledger.importTitle")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <LedgerImportUpload flow={flow} accounts={accounts} />

        {preview && (
          <div className="space-y-4 rounded-md border p-4">
            <LedgerImportPreviewSummary
              preview={preview}
              accountId={resolvedAccountId}
              accountName={accountNameById.get(resolvedAccountId)}
            />

            {flow.unresolvedAccount && <LedgerImportAccountSection flow={flow} accounts={accounts} />}

            <LedgerImportPreviewTable transactions={preview.transactions} />

            <div className="flex flex-wrap justify-end gap-2">
              <Button type="button" variant="outline" onClick={flow.resetFlow}>
                {t("common.cancel")}
              </Button>
              <Button type="button" onClick={flow.handleCommit} disabled={!flow.canCommit || flow.isCommitting}>
                {flow.isCommitting ? t("ledger.committing") : t("ledger.commit")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
