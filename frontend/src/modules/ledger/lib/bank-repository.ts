import { delJson, get, post, put } from "@/lib/api"
import type {
  BankAuthorizeInput,
  BankConnection,
  BankInfo,
  BankSyncSummary,
} from "@/modules/ledger/types"

export async function getBankAspsps(country: string): Promise<BankInfo[]> {
  return get<BankInfo[]>(`/ledger/bank/aspsps?country=${encodeURIComponent(country)}`)
}

export async function getBankConnections(): Promise<BankConnection[]> {
  return get<BankConnection[]>("/ledger/bank/connections")
}

export async function authorizeBank(input: BankAuthorizeInput): Promise<{ url: string }> {
  return post<{ url: string }>("/ledger/bank/connections/authorize", input)
}

export async function disconnectBank(id: string): Promise<{ remoteClosed: boolean }> {
  return delJson<{ remoteClosed: boolean }>(`/ledger/bank/connections/${id}`)
}

export async function mapBankAccount(id: string, uid: string, ledgerAccountId: string): Promise<BankConnection> {
  return put<BankConnection>(`/ledger/bank/connections/${id}/accounts/${encodeURIComponent(uid)}`, { ledgerAccountId })
}

// Removes a mapping; the bank account is then not synced or mapped by IBAN
// until it is mapped again.
export async function unmapBankAccount(id: string, uid: string): Promise<BankConnection> {
  return delJson<BankConnection>(`/ledger/bank/connections/${id}/accounts/${encodeURIComponent(uid)}`)
}

export async function syncBankConnection(id: string): Promise<BankSyncSummary> {
  return post<BankSyncSummary>(`/ledger/bank/connections/${id}/sync`, {})
}
