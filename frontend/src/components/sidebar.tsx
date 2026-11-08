import { useState } from "react"
import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { modules } from "@/modules/registry"
import { useModules } from "@/hooks/use-modules"
import { useInboxAvailable, usePendingDrafts } from "@/hooks/use-drafts"
import { cn } from "@/lib/utils"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { ChevronRight, Home } from "lucide-react"

export function SidebarSection({
  title,
  to,
  isActive,
  defaultOpen = true,
  children,
}: {
  title: string
  to?: string
  isActive?: boolean
  defaultOpen?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="flex items-center">
        <CollapsibleTrigger
          className="flex items-center px-2 py-1.5 text-muted-foreground hover:text-foreground transition-colors"
          aria-label={`Toggle ${title} section`}
        >
          <ChevronRight className={cn("h-3 w-3 transition-transform", open && "rotate-90")} />
        </CollapsibleTrigger>
        {to ? (
          <Link
            to={to}
            className={cn(
              "flex-1 py-1.5 text-xs font-semibold uppercase tracking-wider transition-colors hover:text-foreground",
              isActive ? "text-foreground" : "text-muted-foreground",
            )}
          >
            {title}
          </Link>
        ) : (
          <CollapsibleTrigger className="flex-1 py-1.5 text-left text-xs font-semibold uppercase tracking-wider text-muted-foreground hover:text-foreground transition-colors">
            {title}
          </CollapsibleTrigger>
        )}
      </div>
      <CollapsibleContent>
        <div className="flex flex-col gap-1">
          {children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function InboxLink() {
  const { t } = useTranslation()
  const matchRoute = useMatchRoute()
  const available = useInboxAvailable()
  const { data: drafts } = usePendingDrafts(available)
  if (!available) return null
  const count = drafts?.length ?? 0

  return (
    <Link
      to="/inbox"
      className={cn(
        "flex items-center justify-between rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent",
        matchRoute({ to: "/inbox" }) && "bg-accent",
      )}
    >
      {t("drafts.title")}
      {count > 0 && (
        <span
          className="rounded-full bg-primary px-2 py-0.5 text-xs font-semibold text-primary-foreground"
          aria-label={t("drafts.pendingCount", { count })}
        >
          {count}
        </span>
      )}
    </Link>
  )
}

export function Sidebar() {
  const { t } = useTranslation()
  const matchRoute = useMatchRoute()
  const { isEnabled } = useModules()

  return (
    <aside className="w-64 shrink-0 border-r bg-background">
      <nav className="flex flex-col gap-1 p-4">
        <Link
          to="/"
          className={cn(
            "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent",
            matchRoute({ to: "/" }) && "bg-accent",
          )}
        >
          <Home className="h-4 w-4" />
          {t("home.title")}
        </Link>

        <div className="my-2" />

        {modules.filter((m) => isEnabled(m.id)).map((m) => (
          <m.SidebarSection key={m.id} />
        ))}

        <SidebarSection title={t("nav.general")}>
          <InboxLink />
          <Link
            to="/settings"
            className={cn(
              "rounded-md px-3 py-2 text-sm font-medium transition-colors hover:bg-accent",
              matchRoute({ to: "/settings" }) && "bg-accent",
            )}
          >
            {t("nav.settings")}
          </Link>
        </SidebarSection>
      </nav>
    </aside>
  )
}
