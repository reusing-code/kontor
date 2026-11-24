import { useLedgerTransactions } from "@/modules/ledger/hooks/use-ledger-transactions"
import { LedgerTransactionsTable } from "@/modules/ledger/components/ledger-transactions-table"
import { LedgerTransactionPages, ledgerTransactionPageSize } from "@/modules/ledger/components/ledger-transaction-pages"
import type { LedgerCategory } from "@/modules/ledger/types"

interface LedgerAccountTransactionsProps {
  accountId: string
  categories: LedgerCategory[]
  currency?: string
  // 1-based
  page: number
  onPageChange: (page: number, options?: { replace?: boolean }) => void
}

export function LedgerAccountTransactions({ accountId, categories, currency, page, onPageChange }: LedgerAccountTransactionsProps) {
  const query = useLedgerTransactions(accountId, ledgerTransactionPageSize, (page - 1) * ledgerTransactionPageSize)

  return (
    <LedgerTransactionPages data={query.data} isPending={query.isPending} isPlaceholderData={query.isPlaceholderData} page={page} onPageChange={onPageChange}>
      <LedgerTransactionsTable transactions={query.data?.items ?? []} categories={categories} pageBalance={query.data?.pageBalance} currency={currency} />
    </LedgerTransactionPages>
  )
}
