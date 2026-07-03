import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger"
import { cn } from "@/lib/utils"
import { SidebarSection } from "@/components/sidebar"

export function LedgerSidebarSection() {
  const { t } = useTranslation()
  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const matchRoute = useMatchRoute()

  return (
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
      <Link
        to="/ledger/email-accounts"
        className={cn(
          "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
          matchRoute({ to: "/ledger/email-accounts" }) && "bg-accent font-medium",
        )}
      >
        {t("ledger.email.accounts")}
      </Link>
      <Link
        to="/ledger/email-orders"
        className={cn(
          "rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
          matchRoute({ to: "/ledger/email-orders" }) && "bg-accent font-medium",
        )}
      >
        {t("ledger.email.orders")}
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
  )
}
