import { useState } from "react"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"
import { ExternalLink, FileSearch, Trash2 } from "lucide-react"
import { usePaperlessConfig, usePaperlessLinks, useDetachPaperlessDocument } from "@/hooks/use-paperless"
import type { PaperlessEntityType } from "@/types/paperless"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { PaperlessSearchDialog } from "@/components/paperless/paperless-search-dialog"

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

  if (!configured) return null

  function handleDetach(documentId: number) {
    detach.mutate(documentId, {
      onSuccess: () => toast.success(t("paperless.detached")),
    })
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
            <span className="truncate">{link.title || `#${link.documentId}`}</span>
          </a>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => handleDetach(link.documentId)}
            disabled={detach.isPending}
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
