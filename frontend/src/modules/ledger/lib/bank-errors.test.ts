import { describe, expect, it } from "vitest"
import i18n from "@/i18n"
import { ApiError } from "@/lib/api"
import { bankErrorMessage } from "@/modules/ledger/lib/bank-errors"

const t = i18n.t.bind(i18n)

describe("bankErrorMessage", () => {
  it("names the redirect URL to register", () => {
    const url = "https://kontor.example/api/v1/ledger/bank/callback"
    const error = new ApiError(409, "register it", "redirect_url_not_allowed", {
      code: "redirect_url_not_allowed",
      redirectUrl: url,
    })
    expect(bankErrorMessage(t, error)).toBe(
      `Enable Banking does not accept the redirect URL. Register ${url} as redirect URL in the Enable Banking app.`,
    )
  })

  it("translates a rejected request", () => {
    const error = new ApiError(502, "rejected", "bank_request_rejected")
    expect(bankErrorMessage(t, error)).toBe("Enable Banking rejected the request. The server log has the error code.")
  })

  it("falls back to the endpoint text for an unknown code", () => {
    expect(bankErrorMessage(t, new ApiError(500, "fixed text", "something_new"))).toBe("fixed text")
  })
})
