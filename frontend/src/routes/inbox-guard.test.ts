import { beforeEach, describe, expect, it } from "vitest"
import { isRedirect } from "@tanstack/react-router"
import { queryClient } from "@/lib/query-client"
import { inboxRoute } from "@/routes/router"
import { apiError, mockApi } from "@/test/api-mock"

function routes(llm: boolean, configured: boolean, total: number) {
  return {
    "GET /capabilities": { llm, llmVision: false, bankImport: false },
    "GET /paperless/config": { configured },
    "GET /drafts/summary": { pending: total, total },
  }
}

// Resolves to "open" or to the path the guard redirects to.
async function guard(): Promise<string> {
  try {
    await inboxRoute.options.beforeLoad!({} as never)
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

describe("inbox route guard", () => {
  it("opens with llm and paperless configured without asking for the summary", async () => {
    const api = mockApi(routes(true, true, 0))

    expect(await guard()).toBe("open")
    expect(api.callsTo("GET", "/drafts/summary")).toHaveLength(0)
  })

  it.each([
    [true, false],
    [false, true],
    [false, false],
  ])("redirects home without drafts (llm %s, paperless %s)", async (llm, configured) => {
    mockApi(routes(llm, configured, 0))

    expect(await guard()).toBe("/")
  })

  it.each([
    [true, false],
    [false, true],
    [false, false],
  ])("opens when drafts exist (llm %s, paperless %s)", async (llm, configured) => {
    mockApi(routes(llm, configured, 1))

    expect(await guard()).toBe("open")
  })

  it("fetches the summary fresh, so a draft that just arrived opens the page", async () => {
    queryClient.setQueryData(["drafts", "summary"], { pending: 0, total: 0 })
    const api = mockApi(routes(false, false, 1))

    expect(await guard()).toBe("open")
    expect(api.callsTo("GET", "/drafts/summary")).toHaveLength(1)
  })

  it("does not block the page when a request fails", async () => {
    mockApi({ ...routes(false, false, 0), "GET /drafts/summary": apiError(500, "down") })

    expect(await guard()).toBe("open")
  })
})
