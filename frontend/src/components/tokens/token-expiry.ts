import { addDays, endOfDay, parse } from "date-fns"

export type ExpiryChoice = "30" | "90" | "365" | "custom" | "never"

export function expiryFor(choice: ExpiryChoice, customDate: string): string | undefined {
  switch (choice) {
    case "never":
      return undefined
    case "custom":
      // "yyyy-MM-dd" must be read as a local date: new Date("2026-12-01") is
      // midnight UTC, the previous evening west of Greenwich.
      return endOfDay(parse(customDate, "yyyy-MM-dd", new Date())).toISOString()
    default:
      return addDays(new Date(), Number(choice)).toISOString()
  }
}
