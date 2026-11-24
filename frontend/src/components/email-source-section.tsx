import { useTranslation } from "react-i18next"
import { formatDate } from "@/lib/format"
import { Mail } from "lucide-react"
import { useModules } from "@/hooks/use-modules"
import { useLedgerEmailLinks } from "@/modules/ledger/hooks/use-ledger-email"
import type { LedgerEmailLink, LedgerEmailLinkEntityType } from "@/modules/ledger/types"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"

interface EmailSourceSectionProps {
  entityType: LedgerEmailLinkEntityType
  entityId: string
  variant?: "card" | "inline"
  // show a line instead of nothing when the entity has no source email
  showEmpty?: boolean
}

// mid: is the URL scheme for a Message-ID (RFC 2392); mail clients that
// register it open the message.
function midUrl(messageId: string): string {
  return `mid:${encodeURIComponent(messageId.replace(/^<|>$/g, ""))}`
}

function EmailSourceRow({ link }: { link: LedgerEmailLink }) {
  const { t } = useTranslation()
  const subject = link.subject || t("drafts.sourceEmailNoSubject")
  const details = [link.from, link.date && formatDate(link.date), link.accountName]
    .filter(Boolean)
    .join(" · ")
  return (
    <div className="flex items-start gap-2 text-sm">
      <Mail className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0">
        {link.messageId ? (
          <a href={midUrl(link.messageId)} title={t("drafts.sourceEmailOpen")} className="block truncate text-primary hover:underline">
            {subject}
          </a>
        ) : (
          <p className="truncate">{subject}</p>
        )}
        <p className="truncate text-xs text-muted-foreground">{details}</p>
      </div>
    </div>
  )
}

// EmailSourceSection shows the email an entity was created from through the
// draft inbox. The links are ledger data, so it needs the ledger module.
export function EmailSourceSection({ entityType, entityId, variant = "card", showEmpty = false }: EmailSourceSectionProps) {
  const { t } = useTranslation()
  const { isEnabled, isLoading } = useModules()
  const enabled = !isLoading && isEnabled("ledger")
  const { data: links = [] } = useLedgerEmailLinks(entityType, entityId, enabled)

  if (!enabled || (links.length === 0 && !showEmpty)) return null

  const body = links.length === 0
    ? <p className="text-sm text-muted-foreground">{t("drafts.sourceEmailNone")}</p>
    : (
      <div className="space-y-2">
        {links.map((link) => <EmailSourceRow key={`${link.emailAccountId}/${link.messageId ?? link.createdAt}`} link={link} />)}
      </div>
    )

  if (variant === "inline") {
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium">{t("drafts.sourceEmail")}</p>
        {body}
      </div>
    )
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("drafts.sourceEmail")}</CardTitle>
      </CardHeader>
      <CardContent>{body}</CardContent>
    </Card>
  )
}
