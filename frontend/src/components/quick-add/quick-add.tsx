import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"
import { useNavigate } from "@tanstack/react-router"
import { toast } from "sonner"
import { Sparkles } from "lucide-react"
import { useQuickAddTypes } from "@/hooks/use-quick-add"
import { Button } from "@/components/ui/button"
import { PrefilledCreateDialogs } from "@/components/drafts/prefilled-create-dialogs"
import { entityPath, type PrefillEntry } from "@/components/drafts/draft-input"
import { QuickAddDialog } from "@/components/quick-add/quick-add-dialog"

const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.userAgent)

// QuickAdd is the header entry point (button and Ctrl/Cmd+K). The note and
// attached file are kept until an entry is created. It renders nothing without the LLM or when no purchases/contracts/auto module is on.
export function QuickAdd() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const enabledTypes = useQuickAddTypes()
  const available = enabledTypes.length > 0
  const [open, setOpen] = useState(false)
  const [text, setText] = useState("")
  const [file, setFile] = useState<File | null>(null)
  const [entry, setEntry] = useState<PrefillEntry | null>(null)

  useEffect(() => {
    if (!available) return
    function onKeyDown(e: KeyboardEvent) {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k") {
        e.preventDefault()
        setOpen(true)
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [available])

  if (!available) return null

  async function handleCreate({ type, parentId }: PrefillEntry, create: () => Promise<{ id: string }>) {
    try {
      const { id } = await create()
      setText("")
      setFile(null)
      const path = entityPath(type, id, parentId)
      toast.success(t(`quickAdd.created.${type}`), {
        action: { label: t("quickAdd.open"), onClick: () => void navigate({ to: path }) },
      })
    } catch (err) {
      toast.error(t("drafts.createFailed", { message: (err as Error).message }))
    }
  }

  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)} title={t("quickAdd.title")}>
        <Sparkles />
        <span className="hidden sm:inline">{t("quickAdd.button")}</span>
        <kbd className="hidden rounded border bg-muted px-1 font-mono text-[10px] text-muted-foreground md:inline">
          {isMac ? "⌘K" : "Ctrl K"}
        </kbd>
      </Button>
      <QuickAddDialog
        open={open}
        onOpenChange={setOpen}
        enabledTypes={enabledTypes}
        text={text}
        onTextChange={setText}
        file={file}
        onFileChange={setFile}
        onContinue={(next) => {
          setOpen(false)
          setEntry(next)
        }}
      />
      <PrefilledCreateDialogs entry={entry} onClose={() => setEntry(null)} onCreate={(e, create) => void handleCreate(e, create)} />
    </>
  )
}
