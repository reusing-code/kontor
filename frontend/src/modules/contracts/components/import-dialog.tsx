import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { Upload, Copy, Check, ChevronDown, ChevronRight } from "lucide-react"
import { useImportContracts } from "@/modules/contracts/hooks/use-contracts"
import type { ImportResult } from "@/modules/contracts/lib/contract-repository"
import specText from "../../../../../contract-import-spec.txt?raw"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"

type InputMode = "file" | "paste"

interface ImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function ImportDialog({ open, onOpenChange }: ImportDialogProps) {
  const { t } = useTranslation()
  const importMutation = useImportContracts()
  const fileRef = useRef<HTMLInputElement>(null)
  const [mode, setMode] = useState<InputMode>("file")
  const [file, setFile] = useState<File | null>(null)
  const [jsonText, setJsonText] = useState("")
  const [result, setResult] = useState<ImportResult | null>(null)
  const [specOpen, setSpecOpen] = useState(false)
  const [copied, setCopied] = useState(false)

  const canSubmit = mode === "file" ? !!file : jsonText.trim().length > 0

  function handleClose(v: boolean) {
    if (!v) {
      setFile(null)
      setJsonText("")
      setMode("file")
      setResult(null)
      setSpecOpen(false)
      setCopied(false)
      importMutation.reset()
    }
    onOpenChange(v)
  }

  function handleUpload() {
    let uploadFile: File
    if (mode === "paste") {
      const blob = new Blob([jsonText], { type: "application/json" })
      uploadFile = new File([blob], "import.json", { type: "application/json" })
    } else {
      if (!file) return
      uploadFile = file
    }
    importMutation.mutate(uploadFile, {
      onSuccess: (data) => {
        setResult(data)
        if (data.errors.length === 0) {
          toast.success(t("import.success", { count: data.created }))
        } else {
          toast.warning(t("import.partial", { created: data.created, errors: data.errors.length }))
        }
      },
      onError: (err) => {
        toast.error(err.message)
      },
    })
  }

  function handleCopySpec() {
    navigator.clipboard.writeText(specText).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("import.title")}</DialogTitle>
        </DialogHeader>

        {!result ? (
          <div className="space-y-4">
            <div className="flex gap-1 rounded-md border p-1">
              <button
                type="button"
                className={`flex-1 rounded-sm px-3 py-1.5 text-sm font-medium transition-colors ${mode === "file" ? "bg-muted" : "hover:bg-muted/50"}`}
                onClick={() => setMode("file")}
              >
                {t("import.tabFile")}
              </button>
              <button
                type="button"
                className={`flex-1 rounded-sm px-3 py-1.5 text-sm font-medium transition-colors ${mode === "paste" ? "bg-muted" : "hover:bg-muted/50"}`}
                onClick={() => setMode("paste")}
              >
                {t("import.tabPaste")}
              </button>
            </div>

            {mode === "file" ? (
              <>
                <input
                  ref={fileRef}
                  type="file"
                  accept=".json"
                  className="hidden"
                  onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                />
                <Button
                  type="button"
                  variant="outline"
                  className="w-full justify-start"
                  onClick={() => fileRef.current?.click()}
                >
                  <Upload className="mr-2 h-4 w-4" />
                  {file ? file.name : t("import.selectFile")}
                </Button>
              </>
            ) : (
              <Textarea
                placeholder={t("import.pastePlaceholder")}
                className="min-h-[160px] font-mono text-sm"
                value={jsonText}
                onChange={(e) => setJsonText(e.target.value)}
              />
            )}

            <div className="rounded-md border">
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-sm font-medium hover:bg-muted/50"
                onClick={() => setSpecOpen(!specOpen)}
              >
                {specOpen ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                {t("import.specTitle")}
              </button>
              {specOpen && (
                <div className="border-t">
                  <div className="flex justify-end px-3 pt-2">
                    <Button type="button" variant="ghost" size="sm" onClick={handleCopySpec}>
                      {copied ? <Check className="mr-1 h-3 w-3" /> : <Copy className="mr-1 h-3 w-3" />}
                      {copied ? t("import.copied") : t("import.copySpec")}
                    </Button>
                  </div>
                  <pre className="max-h-64 overflow-auto px-3 pb-3 text-xs text-muted-foreground whitespace-pre-wrap">{specText}</pre>
                </div>
              )}
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => handleClose(false)}>
                {t("common.cancel")}
              </Button>
              <Button
                type="button"
                disabled={!canSubmit || importMutation.isPending}
                onClick={handleUpload}
              >
                {importMutation.isPending ? t("import.uploading") : t("import.upload")}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="space-y-4">
            <p className="text-sm">
              {t("import.createdCount", { count: result.created })}
            </p>
            {result.errors.length > 0 && (
              <div className="space-y-1">
                <p className="text-sm font-medium text-destructive">{t("import.errorsTitle")}</p>
                <ul className="max-h-48 overflow-y-auto space-y-1 text-sm text-muted-foreground">
                  {result.errors.map((e) => (
                    <li key={e.row}>
                      {t("import.rowError", { row: e.row, error: e.error })}
                    </li>
                  ))}
                </ul>
              </div>
            )}
            <DialogFooter>
              <Button type="button" onClick={() => handleClose(false)}>
                {t("common.close")}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
