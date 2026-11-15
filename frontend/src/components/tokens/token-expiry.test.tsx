import { describe, expect, it, vi } from "vitest"
import { fireEvent, screen, within } from "@testing-library/react"
import { expiryFor } from "@/components/tokens/token-expiry"
import { TokensSettingsCard } from "@/components/tokens/tokens-settings-card"
import { mockApi, type ApiCall } from "@/test/api-mock"
import { renderWithProviders } from "@/test/render"

// Sets the process timezone and proves it took effect, so no test below can
// pass because the zone silently stayed UTC.
function useTimezone(zone: string, probe: string, expectedOffsetMinutes: number) {
  vi.stubEnv("TZ", zone)
  const offset = new Date(probe).getTimezoneOffset()
  expect(offset).not.toBe(0)
  expect(offset).toBe(expectedOffsetMinutes)
}

function localParts(iso: string) {
  const d = new Date(iso)
  return [d.getFullYear(), d.getMonth() + 1, d.getDate(), d.getHours(), d.getMinutes(), d.getSeconds(), d.getMilliseconds()]
}

describe("expiryFor", () => {
  it("returns nothing for never", () => {
    expect(expiryFor("never", "")).toBeUndefined()
    expect(expiryFor("never", "2027-03-15")).toBeUndefined()
  })

  it.each([
    ["30", "2026-05-31T10:00:00.000Z"],
    ["90", "2026-07-30T10:00:00.000Z"],
    ["365", "2027-05-01T10:00:00.000Z"],
  ] as const)("adds %s days to now", (choice, expected) => {
    vi.stubEnv("TZ", "UTC")
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-05-01T10:00:00.000Z"))
    expect(expiryFor(choice, "")).toBe(expected)
  })

  it("reads a custom date as the end of that local day west of Greenwich", () => {
    useTimezone("America/Los_Angeles", "2027-03-15T12:00:00Z", 420)

    const iso = expiryFor("custom", "2027-03-15")!
    expect(iso).toBe("2027-03-16T06:59:59.999Z")
    expect(localParts(iso)).toEqual([2027, 3, 15, 23, 59, 59, 999])
    // What the naive parse would give: the evening of the 14th.
    expect(new Date("2027-03-15").getDate()).toBe(14)
  })

  it("reads a custom date as the end of that local day east of Greenwich", () => {
    useTimezone("Pacific/Auckland", "2027-03-15T12:00:00Z", -780)

    const iso = expiryFor("custom", "2027-03-15")!
    expect(iso).toBe("2027-03-15T10:59:59.999Z")
    expect(localParts(iso)).toEqual([2027, 3, 15, 23, 59, 59, 999])
  })

  it("handles a zone with a half-hour offset", () => {
    useTimezone("Asia/Kolkata", "2027-03-15T12:00:00Z", -330)

    expect(expiryFor("custom", "2027-03-15")).toBe("2027-03-15T18:29:59.999Z")
  })

  it("ends the day at 23:59:59.999 on a daylight saving change", () => {
    // 2027-03-14 is the 23 hour day in Los Angeles.
    useTimezone("America/Los_Angeles", "2027-03-14T12:00:00Z", 420)

    const iso = expiryFor("custom", "2027-03-14")!
    expect(iso).toBe("2027-03-15T06:59:59.999Z")
    expect(localParts(iso)).toEqual([2027, 3, 14, 23, 59, 59, 999])
  })
})

describe("TokenCreateDialog custom expiry in a non-UTC timezone", () => {
  async function submitCustomDate(date: string) {
    const api = mockApi({
      "GET /tokens": [],
      "POST /tokens": ({ body }: ApiCall) => ({ id: "new", createdAt: "2026-11-15T02:00:00Z", ...(body as object), token: "kpat_x" }),
    })
    const { user } = renderWithProviders(<TokensSettingsCard />)
    await user.click(screen.getByRole("button", { name: "Create token" }))
    const dialog = await screen.findByRole("dialog", { name: "Create token" })
    await user.type(within(dialog).getByLabelText("Name"), "Dated")
    await user.click(within(dialog).getByRole("combobox", { name: "Expires" }))
    await user.click(await screen.findByRole("option", { name: "Custom date" }))
    const input = within(dialog).getByLabelText("Custom date")
    fireEvent.change(input, { target: { value: date } })
    await user.click(within(dialog).getByRole("button", { name: "Create" }))
    return { api, input }
  }

  it("accepts the local tomorrow and sends the end of that local day", async () => {
    useTimezone("America/Los_Angeles", "2026-11-15T02:00:00Z", 480)
    // 18:00 on the 14th in Los Angeles, already the 15th in UTC.
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-11-15T02:00:00.000Z"))

    const { api, input } = await submitCustomDate("2026-11-15")

    await screen.findByRole("dialog", { name: "Your new token" })
    expect(input).toHaveAttribute("min", "2026-11-15")
    expect(api.callsTo("POST", "/tokens").map((c) => c.body)).toEqual([
      { name: "Dated", scopes: ["read"], expiresAt: "2026-11-16T07:59:59.999Z" },
    ])
  })

  it("rejects the local today even when it is tomorrow in UTC terms", async () => {
    useTimezone("Pacific/Auckland", "2026-11-14T13:00:00Z", -780)
    // 02:00 on the 15th in Auckland, still the 14th in UTC.
    vi.useFakeTimers({ toFake: ["Date"] })
    vi.setSystemTime(new Date("2026-11-14T13:00:00.000Z"))

    const { api, input } = await submitCustomDate("2026-11-15")

    // Below min the browser's own validation blocks the submit.
    expect(input).toHaveAttribute("min", "2026-11-16")
    expect(input).toBeInvalid()
    expect(screen.queryByRole("dialog", { name: "Your new token" })).not.toBeInTheDocument()
    expect(api.callsTo("POST", "/tokens")).toHaveLength(0)
  })
})
