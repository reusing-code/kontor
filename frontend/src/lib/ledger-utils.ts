import { format } from "date-fns"

export function formatLedgerDate(value?: string): string {
  if (!value) return "-"
  return format(new Date(value), "yyyy-MM-dd")
}

export function formatAmountMinor(amountMinor: number, currency: string = "EUR", locale: string = navigator.language): string {
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amountMinor / 100)
}

export function formatSourceType(sourceType: string): string {
  switch (sourceType) {
    case "dkb.csv":
      return "DKB CSV"
    case "comdirect.csv":
      return "comdirect CSV"
    default:
      return sourceType
  }
}
