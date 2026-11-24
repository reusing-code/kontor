import { useTranslation } from "react-i18next"
import { useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger-accounts"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { useAllLedgerTransactions } from "@/modules/ledger/hooks/use-ledger-transactions"
import { selectedAccountIds } from "@/modules/ledger/lib/transaction-filter"
import { LedgerAccountFilter } from "@/modules/ledger/components/ledger-account-filter"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"

interface LedgerAllTransactionsProps {
  // the selection from the URL; absent = all accounts
  accountIds?: string[]
  onAccountIdsChange: (accountIds: string[]) => void
}

export function LedgerAllTransactions({ accountIds, onAccountIdsChange }: LedgerAllTransactionsProps) {
  const { t } = useTranslation()
  const { data: accounts, isSuccess: accountsLoaded } = useLedgerAccounts()
  const { data: categories = [] } = useLedgerCategories()
  const selected = selectedAccountIds(accountIds, (accounts ?? []).map((account) => account.id))
  const query = useAllLedgerTransactions(selected, accountsLoaded)
  const transactions = query.data?.pages.flatMap((page) => page.items) ?? []
  const total = query.data?.pages.at(-1)?.total ?? 0

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <LedgerAccountFilter accounts={accounts ?? []} selected={selected} onChange={onAccountIdsChange} />
        {query.data && (
          <span className="text-sm text-muted-foreground">
            {t("ledger.allTransactions.shown", { shown: transactions.length, count: total })}
          </span>
        )}
      </div>
      <LedgerTransactionsTable
        transactions={transactions}
        categories={categories}
        accounts={accounts ?? []}
        hasMore={query.hasNextPage}
        loadingMore={query.isFetchingNextPage}
        onLoadMore={() => void query.fetchNextPage()}
      />
    </div>
  )
}
