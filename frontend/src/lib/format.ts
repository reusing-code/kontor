import i18n from "@/i18n"

export type DateStyle = "short" | "medium" | "long"

const isoDatePattern = /^(\d{4})-(\d{2})-(\d{2})$/

const moneyFormats = new Map<string, Intl.NumberFormat | null>()
// only for calendar dates, which are formatted in UTC; a formatter keeps the
// timezone it was created in, so local times get a fresh one
const dateFormats = new Map<string, Intl.DateTimeFormat>()

// null: the currency code is unknown to Intl (e.g. one an LLM extracted)
function moneyFormat(language: string, currency: string): Intl.NumberFormat | null {
  const key = `${language}|${currency}`
  if (!moneyFormats.has(key)) {
    try {
      moneyFormats.set(key, new Intl.NumberFormat(language, { style: "currency", currency }))
    } catch {
      moneyFormats.set(key, null)
    }
  }
  return moneyFormats.get(key) ?? null
}

function calendarDateFormat(language: string, style: DateStyle): Intl.DateTimeFormat {
  const key = `${language}|${style}`
  let format = dateFormats.get(key)
  if (!format) {
    format = new Intl.DateTimeFormat(language, { dateStyle: style, timeZone: "UTC" })
    dateFormats.set(key, format)
  }
  return format
}

// formatMoney(-12.5) -> "-€12.50" (en) / "-12,50 €" (de); "-" for a
// non-finite amount, "12.50 XYZ" for a currency code Intl does not know.
export function formatMoney(amount: number, currency = "EUR"): string {
  if (!Number.isFinite(amount)) return "-"
  const format = moneyFormat(i18n.language, currency)
  if (format) return format.format(amount)
  const number = new Intl.NumberFormat(i18n.language, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
  return `${number} ${currency}`
}

// formatMinor(1234) -> "€12.34": amounts in minor units (cents).
export function formatMinor(minor: number, currency = "EUR"): string {
  return formatMoney(minor / 100, currency)
}

// A YYYY-MM-DD string is that calendar day in every timezone (never
// new Date("2026-01-01"), which is UTC midnight); a timestamp or a Date is
// shown in local time. An unparseable string is returned as is, an invalid
// Date as "-".
export function formatDate(value: string | Date, style: DateStyle = "medium"): string {
  if (typeof value === "string" && isoDatePattern.test(value)) {
    const date = calendarDate(value)
    return date ? calendarDateFormat(i18n.language, style).format(date) : value
  }
  return formatLocal(value, { dateStyle: style })
}

export function formatDateTime(value: string | Date, style: DateStyle = "medium"): string {
  return formatLocal(value, { dateStyle: style, timeStyle: "short" })
}

// calendarDate("2026-01-31") is that day at UTC midnight; undefined for a
// day that does not exist, such as "2026-02-30".
function calendarDate(value: string): Date | undefined {
  const [year, month, day] = value.split("-").map(Number)
  const date = new Date(Date.UTC(year, month - 1, day))
  return date.getUTCFullYear() === year && date.getUTCMonth() === month - 1 && date.getUTCDate() === day ? date : undefined
}

function formatLocal(value: string | Date, options: Intl.DateTimeFormatOptions): string {
  const date = typeof value === "string" ? new Date(value) : value
  if (Number.isNaN(date.getTime())) return typeof value === "string" ? value : "-"
  return new Intl.DateTimeFormat(i18n.language, options).format(date)
}
