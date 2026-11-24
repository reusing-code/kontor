import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { ledgerKeys } from "@/modules/ledger/keys"
import {
  getLedgerCashFlowAnalysis,
  getLedgerNetWorthAnalysis,
  getLedgerRankingAnalysis,
  getLedgerSpendingAnalysis,
} from "@/modules/ledger/lib/ledger-repository"
import type { LedgerAnalysisRange } from "@/modules/ledger/types"

// The analyses keep the previous data on screen while another range loads.
export function useLedgerSpendingAnalysis(range: LedgerAnalysisRange, parentCategoryId?: string) {
  return useQuery({
    queryKey: ledgerKeys.spending(range, parentCategoryId),
    queryFn: () => getLedgerSpendingAnalysis(range, parentCategoryId),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerCashFlowAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.cashFlow(range),
    queryFn: () => getLedgerCashFlowAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerRankingAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.ranking(range),
    queryFn: () => getLedgerRankingAnalysis(range),
    placeholderData: keepPreviousData,
  })
}

export function useLedgerNetWorthAnalysis(range: LedgerAnalysisRange) {
  return useQuery({
    queryKey: ledgerKeys.netWorth(range),
    queryFn: () => getLedgerNetWorthAnalysis(range),
    placeholderData: keepPreviousData,
  })
}
