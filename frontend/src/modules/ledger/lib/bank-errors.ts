import type { TFunction } from "i18next"
import { ApiError } from "@/lib/api"

// Fixed endpoint codes get a translated message; anything else shows the
// endpoint's own (fixed, English) text. redirect_url_not_allowed carries the
// URL to register.
export function bankErrorMessage(t: TFunction, error: Error): string {
  if (error instanceof ApiError && error.code) {
    const redirectUrl = typeof error.body.redirectUrl === "string" ? error.body.redirectUrl : ""
    return t(`ledger.bankImport.errors.${error.code}`, { defaultValue: error.message, redirectUrl })
  }
  return error.message
}
