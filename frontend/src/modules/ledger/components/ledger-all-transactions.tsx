import { useLedgerAccounts } from "@/modules/ledger/hooks/use-ledger-accounts"
import { useLedgerCategories } from "@/modules/ledger/hooks/use-ledger-categories"
import { useAllLedgerTransactions } from "@/modules/ledger/hooks/use-ledger-transactions"
import { selectedAccountIds } from "@/modules/ledger/lib/transaction-filter"
import { LedgerAccountFilter } from "@/modules/ledger/components/ledger-account-filter"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"
import { LedgerTransactionPages, ledgerTransactionPageSize } from "@/modules/ledger/components/ledger-transaction-pages"

interface LedgerAllTransactionsProps {
  // the selection from the URL; absent = all accounts
  accountIds?: string[]
  onAccountIdsChange: (accountIds: string[]) => void
  // 1-based
  page: number
  onPageChange: (page: number, options?: { replace?: boolean }) => void
}

export function LedgerAllTransactions({ accountIds, onAccountIdsChange, page, onPageChange }: LedgerAllTransactionsProps) {
  const { data: accounts, isSuccess: accountsLoaded } = useLedgerAccounts()
  const { data: categories = [] } = useLedgerCategories()
  const selected = selectedAccountIds(accountIds, (accounts ?? []).map((account) => account.id))
  const query = useAllLedgerTransactions(selected, (page - 1) * ledgerTransactionPageSize, accountsLoaded)

  return (
    <LedgerTransactionPages
      data={query.data}
      isPending={query.isPending}
      isPlaceholderData={query.isPlaceholderData}
      page={page}
      onPageChange={onPageChange}
      toolbar={<LedgerAccountFilter accounts={accounts ?? []} selected={selected} onChange={onAccountIdsChange} />}
    >
      <LedgerTransactionsTable transactions={query.data?.items ?? []} categories={categories} accounts={accounts ?? []} />
    </LedgerTransactionPages>
  )
}
