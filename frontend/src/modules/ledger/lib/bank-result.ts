// The callback redirects to the result page with fixed codes only. Anything
// else in the query string is ignored and never shown.
export const bankResultReasons = [
  "denied",
  "bank_error",
  "invalid_state",
  "missing_code",
  "restricted",
  "failed",
  "disabled",
] as const

export type BankResultReason = (typeof bankResultReasons)[number]

export interface BankResult {
  bank: "connected" | "failed"
  reason?: BankResultReason
}

export function parseBankResult(search: Record<string, unknown>): BankResult {
  if (search.bank === "connected") return { bank: "connected" }
  const reason = bankResultReasons.find((r) => r === search.reason)
  return { bank: "failed", reason }
}
