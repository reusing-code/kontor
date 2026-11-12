import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { CheckIcon, CopyIcon } from "lucide-react"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

interface TokenRevealDialogProps {
  // The raw token, or null while the dialog is closed. It is only kept in
  // the caller's component state and cleared on close.
  token: string | null
  onClose: () => void
}

export function TokenRevealDialog({ token, onClose }: TokenRevealDialogProps) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  async function handleCopy() {
    if (!token) return
    try {
      await navigator.clipboard.writeText(token)
      setCopied(true)
      toast.success(t("tokens.copied"))
    } catch {
      toast.error(t("tokens.copyFailed"))
    }
  }

  function handleOpenChange(open: boolean) {
    if (!open) {
      setCopied(false)
      onClose()
    }
  }

  return (
    <Dialog open={token !== null} onOpenChange={handleOpenChange}>
      {/* only the Close button dismisses it: the token is shown once */}
      <DialogContent onInteractOutside={(e) => e.preventDefault()} onEscapeKeyDown={(e) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>{t("tokens.revealTitle")}</DialogTitle>
          <DialogDescription>{t("tokens.revealDescription")}</DialogDescription>
        </DialogHeader>
        <div className="flex gap-2">
          <Input readOnly value={token ?? ""} aria-label={t("tokens.tokenValue")} className="font-mono" onFocus={(e) => e.target.select()} />
          <Button type="button" variant="outline" size="icon" onClick={handleCopy} aria-label={t("tokens.copy")}>
            {copied ? <CheckIcon className="h-4 w-4" /> : <CopyIcon className="h-4 w-4" />}
          </Button>
        </div>
        <p className="text-sm font-medium text-destructive" role="alert">
          {t("tokens.revealWarning")}
        </p>
        <DialogFooter>
          <Button type="button" onClick={() => handleOpenChange(false)}>
            {t("common.close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
