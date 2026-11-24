import { useState } from "react"
import { useTranslation } from "react-i18next"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { useForm, type Resolver } from "react-hook-form"
import { toast } from "sonner"
import { useLedgerCommitImport, useLedgerPreviewImport } from "@/modules/ledger/hooks/use-ledger-imports"
import { defaultLedgerAccountInput } from "@/modules/ledger/lib/ledger-repository"
import { ledgerAccountInputSchema, type LedgerAccount, type LedgerAccountInput, type LedgerPreviewResult, type LedgerFileSourceType } from "@/modules/ledger/types"

export type LedgerImportMode = "existing" | "new"

export type LedgerImportFlow = ReturnType<typeof useLedgerImportFlow>

// Upload, preview, account resolution and commit of one CSV import.
export function useLedgerImportFlow(accounts: LedgerAccount[]) {
  const { t } = useTranslation()
  const previewMutation = useLedgerPreviewImport()
  const commitMutation = useLedgerCommitImport()
  const [file, setFile] = useState<File | null>(null)
  const [sourceType, setSourceType] = useState<LedgerFileSourceType>("dkb.csv")
  const [selectedAccountId, setSelectedAccountId] = useState<string>("")
  const [preview, setPreview] = useState<LedgerPreviewResult | null>(null)
  const [mode, setMode] = useState<LedgerImportMode>("existing")
  // remounts the uncontrolled file input on reset, so the same file can be picked again
  const [fileInputKey, setFileInputKey] = useState(0)

  const newAccountForm = useForm<LedgerAccountInput>({
    resolver: standardSchemaResolver(ledgerAccountInputSchema) as Resolver<LedgerAccountInput>,
    defaultValues: defaultLedgerAccountInput(),
  })

  const unresolvedAccount = preview && !preview.accountId
  const canPreview = !!file
  const canCommit = !!preview && (mode === "new" || selectedAccountId || preview.accountId)

  function resetFlow() {
    setFile(null)
    setFileInputKey((key) => key + 1)
    setSelectedAccountId("")
    setPreview(null)
    setMode("existing")
    newAccountForm.reset(defaultLedgerAccountInput())
    previewMutation.reset()
    commitMutation.reset()
  }

  function handlePreview() {
    if (!file) return
    previewMutation.mutate(
      {
        file,
        sourceType,
        accountId: selectedAccountId || undefined,
      },
      {
        onSuccess: (result) => {
          setPreview(result)
          setSelectedAccountId(result.accountId ?? selectedAccountId)
          newAccountForm.reset(defaultLedgerAccountInput(result.iban, result.bankName))
          if (!result.accountId) {
            setMode(accounts.length > 0 ? "existing" : "new")
          }
          toast.success(t("ledger.previewReady"))
        },
      },
    )
  }

  function handleCommit() {
    if (!preview) return

    if (mode === "new") {
      void newAccountForm.handleSubmit((data) => {
        commitMutation.mutate(
          {
            previewId: preview.previewId,
            data: { newAccount: data },
          },
          {
            onSuccess: (result) => {
              toast.success(t("ledger.importCommitted", { count: result.importedRows }))
              resetFlow()
            },
          },
        )
      })()
      return
    }

    commitMutation.mutate(
      {
        previewId: preview.previewId,
        data: { accountId: selectedAccountId || preview.accountId },
      },
      {
        onSuccess: (result) => {
          toast.success(t("ledger.importCommitted", { count: result.importedRows }))
          resetFlow()
        },
      },
    )
  }

  return {
    setFile,
    fileInputKey,
    sourceType,
    setSourceType,
    selectedAccountId,
    setSelectedAccountId,
    preview,
    mode,
    setMode,
    newAccountForm,
    unresolvedAccount,
    canPreview,
    canCommit,
    isPreviewing: previewMutation.isPending,
    isCommitting: commitMutation.isPending,
    handlePreview,
    handleCommit,
    resetFlow,
  }
}
