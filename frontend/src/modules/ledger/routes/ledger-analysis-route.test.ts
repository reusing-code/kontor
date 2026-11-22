import { beforeEach, describe, expect, it } from "vitest"
import { isRedirect } from "@tanstack/react-router"
import { queryClient } from "@/lib/query-client"
import { ledgerAnalysisRoute, ledgerRoutes } from "@/modules/ledger/routes/routes"
import { apiError, mockApi } from "@/test/api-mock"

const settings = (enabledModules: string[]) => ({ renewalDays: 90, reminderFrequency: "disabled", enabledModules })

// Resolves to "open" or to the path the guard redirects to.
async function guard(): Promise<string> {
  try {
    await ledgerAnalysisRoute.options.beforeLoad!({} as never)
    return "open"
  } catch (error) {
    if (isRedirect(error)) return String(error.options.to)
    throw error
  }
}

beforeEach(() => {
  queryClient.clear()
  queryClient.setDefaultOptions({ queries: { retry: false } })
})

describe("ledger analysis route", () => {
  it("is registered with the ledger routes under /ledger/analysis", () => {
    expect(ledgerRoutes).toContain(ledgerAnalysisRoute)
    const paths = ledgerRoutes.map((route) => (route.options as { path?: string }).path)
    expect(paths[ledgerRoutes.indexOf(ledgerAnalysisRoute)]).toBe("/ledger/analysis")
    expect(paths.filter((path) => path === "/ledger/analysis")).toHaveLength(1)
  })

  it("opens while the ledger module is enabled", async () => {
    mockApi({ "GET /settings": settings(["contracts", "ledger"]) })

    expect(await guard()).toBe("open")
  })

  it("redirects home while the ledger module is disabled, without asking for an analysis", async () => {
    const api = mockApi({ "GET /settings": settings(["contracts", "purchases", "auto"]) })

    expect(await guard()).toBe("/")
    expect(api.calls.filter((call) => call.path.startsWith("/ledger/"))).toEqual([])
  })

  it("does not block the page when the settings cannot be loaded", async () => {
    mockApi({ "GET /settings": apiError(500, "down") })

    expect(await guard()).toBe("open")
  })
})
