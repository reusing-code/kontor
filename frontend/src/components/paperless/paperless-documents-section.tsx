import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ExternalLink, FileSearch, Trash2 } from "lucide-react"
import { usePaperlessConfig, usePaperlessLinks, useDetachPaperlessDocument } from "@/hooks/use-paperless"
import type { PaperlessEntityType, PaperlessLink } from "@/types/paperless"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { PaperlessSearchDialog } from "@/components/paperless/paperless-search-dialog"
import { DeleteConfirmDialog } from "@/components/delete-confirm-dialog"

interface PaperlessDocumentsSectionProps {
  entityType: PaperlessEntityType
  entityId: string
  entityUrl: string
  variant?: "card" | "inline"
}

export function PaperlessDocumentsSection({
  entityType,
  entityId,
  entityUrl,
  variant = "card",
}: PaperlessDocumentsSectionProps) {
  const { t } = useTranslation()
  const { data: config } = usePaperlessConfig()
  const configured = config?.configured ?? false
  const { data: links } = usePaperlessLinks(entityType, entityId, configured)
  const detach = useDetachPaperlessDocument(entityType, entityId)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [detaching, setDetaching] = useState<PaperlessLink | null>(null)

  if (!configured) return null

  async function handleDetach() {
    if (!detaching) return
    await detach.mutateAsync(detaching.documentId)
    toast.success(t("paperless.detached"))
  }

  const body = (
    <div className="space-y-2">
      {(links ?? []).map((link) => (
        <div key={link.documentId} className="flex items-center gap-2">
          <a
            href={`${config?.baseUrl}/documents/${link.documentId}/details`}
            target="_blank"
            rel="noopener noreferrer"
            className="flex min-w-0 flex-1 items-center gap-2 text-sm text-primary hover:underline"
            title={t("paperless.openInPaperless")}
          >
            <ExternalLink className="h-4 w-4 shrink-0" />
            <span className="truncate">{documentTitle(link)}</span>
          </a>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setDetaching(link)}
            disabled={detach.isPending}
            aria-label={t("paperless.detachDocument", { title: documentTitle(link) })}
            title={t("paperless.detach")}
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      ))}
      {(links ?? []).length === 0 && <p className="text-sm text-muted-foreground">{t("paperless.noDocuments")}</p>}
      <Button variant="outline" size="sm" onClick={() => setDialogOpen(true)}>
        <FileSearch className="mr-2 h-4 w-4" />
        {t("paperless.attachDocuments")}
      </Button>
      <PaperlessSearchDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        entityType={entityType}
        entityId={entityId}
        entityUrl={entityUrl}
        existingDocIds={(links ?? []).map((l) => l.documentId)}
      />
      <DeleteConfirmDialog
        open={detaching !== null}
        onOpenChange={(open) => { if (!open) setDetaching(null) }}
        description={t("paperless.detachConfirm", { title: detaching ? documentTitle(detaching) : "" })}
        actionLabel={t("paperless.detach")}
        onConfirm={handleDetach}
      />
    </div>
  )

  if (variant === "inline") {
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium">{t("paperless.documents")}</p>
        {body}
      </div>
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("paperless.documents")}</CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}

function documentTitle(link: PaperlessLink): string {
  return link.title || `#${link.documentId}`
}
