import { useState } from "react"
import type { MouseEvent } from "react"
import { useTranslation } from "react-i18next"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

interface DeleteConfirmDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  description: string
  // A returned promise keeps the dialog open (buttons disabled) until it
  // settles; it closes only once it resolves.
  onConfirm: () => void | Promise<unknown>
  actionLabel?: string
}

export function DeleteConfirmDialog({
  open,
  onOpenChange,
  description,
  onConfirm,
  actionLabel,
}: DeleteConfirmDialogProps) {
  const { t } = useTranslation()
  const [pending, setPending] = useState(false)

  async function handleConfirm(event: MouseEvent) {
    const result = onConfirm()
    if (!(result instanceof Promise)) return
    event.preventDefault()
    setPending(true)
    try {
      await result
      onOpenChange(false)
    } catch {
      // the global mutation error toast reports it
    } finally {
      setPending(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={(next) => { if (!pending) onOpenChange(next) }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t("common.confirm")}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("common.cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={(event) => void handleConfirm(event)} disabled={pending}>
            {actionLabel ?? t("common.delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
