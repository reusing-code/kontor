import { describe, expect, it } from "vitest"
import { renderHook, waitFor } from "@testing-library/react"
import { useInboxAvailable } from "@/hooks/use-drafts"
import { apiError, mockApi } from "@/test/api-mock"
import { createTestQueryClient, queryWrapper } from "@/test/render"

interface Server {
  llm: boolean
  configured: boolean
  total: number
  pending?: number
}

function routes({ llm, configured, total, pending = total }: Server) {
  return {
    "GET /capabilities": { llm, llmVision: false, bankImport: false },
    "GET /paperless/config": { configured },
    "GET /drafts/summary": { pending, total },
  }
}

async function available(server: Record<string, unknown>) {
  const api = mockApi(server)
  const queryClient = createTestQueryClient()
  const { result } = renderHook(() => useInboxAvailable(), { wrapper: queryWrapper(queryClient) })
  // Hidden until the queries answer.
  expect(result.current).toBe(false)
  await waitFor(() => expect(api.calls).toHaveLength(3))
  await waitFor(() => expect(queryClient.isFetching()).toBe(0))
  return result.current
}

describe("useInboxAvailable", () => {
  it.each<[string, Server, boolean]>([
    ["llm and paperless configured, no drafts", { llm: true, configured: true, total: 0 }, true],
    ["llm without paperless, no drafts", { llm: true, configured: false, total: 0 }, false],
    ["paperless without llm, no drafts", { llm: false, configured: true, total: 0 }, false],
    ["neither, no drafts", { llm: false, configured: false, total: 0 }, false],
    ["neither, a pending draft", { llm: false, configured: false, total: 1 }, true],
    ["neither, only decided drafts", { llm: false, configured: false, total: 3, pending: 0 }, true],
    ["llm without paperless, a draft", { llm: true, configured: false, total: 2 }, true],
    ["paperless without llm, a draft", { llm: false, configured: true, total: 2 }, true],
  ])("%s", async (_name, server, expected) => {
    expect(await available(routes(server))).toBe(expected)
  })

  it("stays available through drafts when capabilities and paperless fail", async () => {
    const server = {
      ...routes({ llm: true, configured: true, total: 1 }),
      "GET /capabilities": apiError(500, "down"),
      "GET /paperless/config": apiError(500, "down"),
    }
    expect(await available(server)).toBe(true)
  })

  it("stays available through llm and paperless when the summary fails", async () => {
    const server = { ...routes({ llm: true, configured: true, total: 0 }), "GET /drafts/summary": apiError(500, "down") }
    expect(await available(server)).toBe(true)
  })

  it("is hidden when everything fails", async () => {
    const down = apiError(500, "down")
    expect(await available({ "GET /capabilities": down, "GET /paperless/config": down, "GET /drafts/summary": down })).toBe(false)
  })
})
