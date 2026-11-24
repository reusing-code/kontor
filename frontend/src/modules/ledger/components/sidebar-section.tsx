import { Link, useMatchRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"
import { useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger-accounts"
import { useCapabilities } from "@/hooks/use-capabilities"
import { SidebarLink, SidebarSection, SidebarSubSection } from "@/components/sidebar"
import { sidebarLinkClass } from "@/components/sidebar-link-class"

// The data pages come first, then the accounts (there can be many, so they
// start collapsed) and last the administrative pages.
export function LedgerSidebarSection() {
  const { t } = useTranslation()
  const { data: ledgerAccounts = [] } = useLedgerAccounts()
  const { bankImport } = useCapabilities()
  const matchRoute = useMatchRoute()

  const accountActive = !!matchRoute({ to: "/ledger/accounts/$accountId" })
  const categoriesActive = !!matchRoute({ to: "/ledger/categories" })
  const bankActive = bankImport && !!matchRoute({ to: "/ledger/bank-connections", fuzzy: true })
  const emailAccountsActive = !!matchRoute({ to: "/ledger/email-accounts" })

  return (
    <SidebarSection
      title={t("nav.ledger")}
      to="/ledger"
      isActive={!!matchRoute({ to: "/ledger", fuzzy: true })}
    >
      <SidebarLink to="/ledger/transactions" active={!!matchRoute({ to: "/ledger/transactions" })}>
        {t("ledger.allTransactions.title")}
      </SidebarLink>
      <SidebarLink to="/ledger/review" active={!!matchRoute({ to: "/ledger/review" })}>
        {t("ledger.reviewQueue")}
      </SidebarLink>
      <SidebarLink to="/ledger/analysis" active={!!matchRoute({ to: "/ledger/analysis" })}>
        {t("ledger.analysis.title")}
      </SidebarLink>
      <SidebarLink to="/ledger/email-orders" active={!!matchRoute({ to: "/ledger/email-orders" })}>
        {t("ledger.email.orders")}
      </SidebarLink>
      {ledgerAccounts.length > 0 && (
        <SidebarSubSection title={t("nav.ledgerAccounts")} containsActive={accountActive} defaultOpen={false}>
          {ledgerAccounts.map((account) => (
            <Link
              key={account.id}
              to="/ledger/accounts/$accountId"
              params={{ accountId: account.id }}
              className={sidebarLinkClass(!!matchRoute({ to: "/ledger/accounts/$accountId", params: { accountId: account.id } }))}
            >
              {account.name}
            </Link>
          ))}
        </SidebarSubSection>
      )}
      <SidebarSubSection title={t("nav.administration")} containsActive={categoriesActive || bankActive || emailAccountsActive}>
        <SidebarLink to="/ledger/categories" active={categoriesActive}>
          {t("ledger.categories")}
        </SidebarLink>
        {bankImport && (
          <SidebarLink to="/ledger/bank-connections" active={bankActive}>
            {t("ledger.bankImport.title")}
          </SidebarLink>
        )}
        <SidebarLink to="/ledger/email-accounts" active={emailAccountsActive}>
          {t("ledger.email.accounts")}
        </SidebarLink>
      </SidebarSubSection>
    </SidebarSection>
  )
}
