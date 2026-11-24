import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { Paperclip } from "lucide-react"
import type { PaperlessLink } from "@/types/paperless"
import { Button } from "@/components/ui/button"

interface PaperlessLinksIndicatorProps {
  links: PaperlessLink[] | undefined
  // Either opens the documents in place or links to the entity's page.
  onOpen?: () => void
  to?: string
}

function linkTitles(links: PaperlessLink[]): string {
  return links.map((link) => link.title || `#${link.documentId}`).join("\n")
}

export function PaperlessLinksIndicator({ links, onOpen, to }: PaperlessLinksIndicatorProps) {
  const { t } = useTranslation()
  if (!links || links.length === 0) return null

  const label = t("paperless.linkedDocuments", { count: links.length })
  const content = (
    <>
      <Paperclip />
      <span className="tabular-nums">{links.length}</span>
    </>
  )
  const props = {
    variant: "ghost" as const,
    size: "sm" as const,
    className: "h-8 px-2 text-muted-foreground",
    "aria-label": label,
    title: linkTitles(links),
  }

  if (to) {
    return (
      <Button asChild {...props}>
        <Link to={to} onClick={(e) => e.stopPropagation()}>
          {content}
        </Link>
      </Button>
    )
  }
  return (
    <Button
      type="button"
      {...props}
      onClick={(e) => {
        e.stopPropagation()
        onOpen?.()
      }}
    >
      {content}
    </Button>
  )
}
