// The backend refuses balances beyond 10^15 minor units.
export const maxBalanceMinor = 1_000_000_000_000_000

// parseBalanceInput reads a signed amount ("1234", "-12,5", "+1234.56"; no
// thousands separators) into minor units; zero is a valid balance.
export function parseBalanceInput(text: string): number | undefined {
  const match = /^([+-])?(\d{1,13})(?:[.,](\d{1,2}))?$/.exec(text.trim().replace(/\s/g, ""))
  if (!match) return undefined
  const minor = Number(match[2]) * 100 + Number((match[3] ?? "").padEnd(2, "0"))
  if (minor > maxBalanceMinor) return undefined
  return match[1] === "-" && minor !== 0 ? -minor : minor
}

export function todayISO(now: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, "0")
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`
}

// hasDrift: the newest balance does not match the one before it plus the
// transactions in between.
export function hasDrift(driftMinor: number | undefined): driftMinor is number {
  return driftMinor !== undefined && driftMinor !== 0
}
