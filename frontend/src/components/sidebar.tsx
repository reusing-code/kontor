import { useState } from "react"
import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useCategories } from "@/hooks/use-categories"
import { useLedgerAccounts } from "@/hooks/use-ledger"
import { useVehicles } from "@/hooks/use-vehicles"
import { cn } from "@/lib/utils"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { ChevronRight, Home } from "lucide-react"

function SidebarSection({
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

export function Sidebar() {
  const { t } = useTranslation()
  const { data: contractCategories = [] } = useCategories("contracts")
  const { data: purchaseCategories = [] } = useCategories("purchases")
  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const { data: vehicles = [] } = useVehicles()
  const matchRoute = useMatchRoute()

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

        <SidebarSection
          title={t("nav.contracts")}
          to="/contracts"
          isActive={!!matchRoute({ to: "/contracts", fuzzy: true })}
        >
          {contractCategories.map((category) => {
            const active = matchRoute({
              to: "/contracts/categories/$categoryId",
              params: { categoryId: category.id },
            })
            return (
              <Link
                key={category.id}
                to="/contracts/categories/$categoryId"
                params={{ categoryId: category.id }}
                className={cn(
                  "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
                  active && "bg-accent font-medium",
                )}
              >
                {category.nameKey ? t(category.nameKey) : category.name}
              </Link>
            )
          })}
        </SidebarSection>

        <SidebarSection
          title={t("nav.purchases")}
          to="/purchases"
          isActive={!!matchRoute({ to: "/purchases", fuzzy: true })}
        >
          {purchaseCategories.map((category) => {
            const active = matchRoute({
              to: "/purchases/categories/$categoryId",
              params: { categoryId: category.id },
            })
            return (
              <Link
                key={category.id}
                to="/purchases/categories/$categoryId"
                params={{ categoryId: category.id }}
                className={cn(
                  "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
                  active && "bg-accent font-medium",
                )}
              >
                {category.nameKey ? t(category.nameKey) : category.name}
              </Link>
            )
          })}
        </SidebarSection>

        <SidebarSection
          title={t("nav.ledger")}
          to="/ledger"
          isActive={!!matchRoute({ to: "/ledger", fuzzy: true })}
        >
          <Link
            to="/ledger/review"
            className={cn(
              "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
              matchRoute({ to: "/ledger/review" }) && "bg-accent font-medium",
            )}
          >
            {t("ledger.reviewQueue")}
          </Link>
          <Link
            to="/ledger/categories"
            className={cn(
              "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
              matchRoute({ to: "/ledger/categories" }) && "bg-accent font-medium",
            )}
          >
            {t("ledger.categories")}
          </Link>
          {ledgerAccounts.map((account) => {
            const active = matchRoute({
              to: "/ledger/accounts/$accountId",
              params: { accountId: account.id },
            })
            return (
              <Link
                key={account.id}
                to="/ledger/accounts/$accountId"
                params={{ accountId: account.id }}
                className={cn(
                  "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
                  active && "bg-accent font-medium",
                )}
              >
                {account.name}
              </Link>
            )
          })}
        </SidebarSection>

        <SidebarSection
          title={t("nav.auto")}
          to="/auto"
          isActive={!!matchRoute({ to: "/auto", fuzzy: true })}
        >
          {vehicles.map((vehicle) => {
            const active = matchRoute({
              to: "/auto/vehicles/$vehicleId",
              params: { vehicleId: vehicle.id },
            })
            return (
              <Link
                key={vehicle.id}
                to="/auto/vehicles/$vehicleId"
                params={{ vehicleId: vehicle.id }}
                className={cn(
                  "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
                  active && "bg-accent font-medium",
                )}
              >
                {vehicle.name}
              </Link>
            )
          })}
        </SidebarSection>

        <SidebarSection title={t("nav.general")}>
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
