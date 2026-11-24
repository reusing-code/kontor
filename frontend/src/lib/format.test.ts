import { afterEach, describe, expect, it, vi } from "vitest"
import i18n from "@/i18n"
import { formatDate, formatDateTime, formatMinor, formatMoney } from "@/lib/format"

// Intl separates the amount and the currency sign with a no-break space
const plain = (value: string) => value.replace(/[  ]/g, " ")

afterEach(async () => {
  await i18n.changeLanguage("en")
})

describe("formatMoney", () => {
  it("formats amounts in English", () => {
    expect(formatMoney(1000)).toBe("€1,000.00")
    expect(formatMoney(-12.5)).toBe("-€12.50")
    expect(formatMoney(0)).toBe("€0.00")
    expect(formatMoney(12.5, "USD")).toBe("$12.50")
    expect(formatMoney(1234, "JPY")).toBe("¥1,234")
  })

  it("formats amounts in German", async () => {
    await i18n.changeLanguage("de")
    expect(plain(formatMoney(1000))).toBe("1.000,00 €")
    expect(plain(formatMoney(-12.5))).toBe("-12,50 €")
    expect(plain(formatMoney(12.5, "USD"))).toBe("12,50 $")
  })

  it("follows a language change", async () => {
    expect(formatMoney(1.5)).toBe("€1.50")
    await i18n.changeLanguage("de")
    expect(plain(formatMoney(1.5))).toBe("1,50 €")
    await i18n.changeLanguage("en")
    expect(formatMoney(1.5)).toBe("€1.50")
  })

  it("keeps an unknown currency code next to the number", () => {
    expect(formatMoney(12.5, "???")).toBe("12.50 ???")
  })

  it("shows a dash for an amount that is no number", () => {
    expect(formatMoney(Number.NaN)).toBe("-")
    expect(formatMoney(Number.POSITIVE_INFINITY)).toBe("-")
  })
})

describe("formatMinor", () => {
  it("formats minor units", async () => {
    expect(formatMinor(123456)).toBe("€1,234.56")
    expect(formatMinor(-5)).toBe("-€0.05")
    expect(formatMinor(1999, "GBP")).toBe("£19.99")
    await i18n.changeLanguage("de")
    expect(plain(formatMinor(-123456))).toBe("-1.234,56 €")
  })
})

describe("formatDate", () => {
  it("formats a calendar date in every style", async () => {
    expect(formatDate("2026-03-01")).toBe("Mar 1, 2026")
    expect(formatDate("2026-03-01", "short")).toBe("3/1/26")
    expect(formatDate("2026-03-01", "long")).toBe("March 1, 2026")
    await i18n.changeLanguage("de")
    expect(formatDate("2026-03-01")).toBe("01.03.2026")
    expect(formatDate("2026-03-01", "short")).toBe("01.03.26")
    expect(formatDate("2026-03-01", "long")).toBe("1. März 2026")
  })

  it.each([
    ["America/Los_Angeles", 480],
    ["Pacific/Kiritimati", -840],
  ])("keeps the calendar day of a date-only string in %s", (tz, offset) => {
    vi.stubEnv("TZ", tz)
    expect(new Date(2026, 0, 1).getTimezoneOffset()).toBe(offset)
    expect(formatDate("2026-01-01")).toBe("Jan 1, 2026")
    expect(formatDate("2025-12-31")).toBe("Dec 31, 2025")
  })

  it("shows a timestamp and a Date in local time", () => {
    vi.stubEnv("TZ", "Europe/Berlin")
    expect(new Date(2026, 0, 1).getTimezoneOffset()).toBe(-60)
    expect(formatDate("2026-03-01T23:30:00Z")).toBe("Mar 2, 2026")
    expect(formatDate(new Date(2026, 2, 1, 0, 30))).toBe("Mar 1, 2026")
  })

  it("returns an unparseable string as is and a dash for an invalid Date", () => {
    expect(formatDate("soon")).toBe("soon")
    expect(formatDate("2026-02-30")).toBe("2026-02-30")
    expect(formatDate("")).toBe("")
    expect(formatDate(new Date(Number.NaN))).toBe("-")
  })
})

describe("formatDateTime", () => {
  it("adds the local time", async () => {
    vi.stubEnv("TZ", "Europe/Berlin")
    expect(new Date(2026, 0, 1).getTimezoneOffset()).toBe(-60)
    expect(plain(formatDateTime("2026-03-01T13:05:00Z"))).toBe("Mar 1, 2026, 2:05 PM")
    await i18n.changeLanguage("de")
    expect(formatDateTime("2026-03-01T13:05:00Z")).toBe("01.03.2026, 14:05")
  })

  it("returns an unparseable string as is", () => {
    expect(formatDateTime("later")).toBe("later")
  })
})
