import { describe, expect, it, vi } from "vitest"
import { ApiError } from "@/lib/api"
import { acceptDraftsBatch, draftBatchChunkSize } from "@/lib/drafts-repository"
import { apiError, mockApi, type ApiCall } from "@/test/api-mock"

const baseUrl = "https://kontor.example"

function ids(count: number): string[] {
  return Array.from({ length: count }, (_, i) => `d${i + 1}`)
}

function acceptAll({ body }: ApiCall) {
  return { results: (body as { ids: string[] }).ids.map((id) => ({ id, status: "accepted", entityId: `e-${id}` })) }
}

function sentIds(calls: ApiCall[]): string[][] {
  return calls.map((call) => (call.body as { ids: string[] }).ids)
}

describe("acceptDraftsBatch", () => {
  it("sends the drafts in chunks of 25 with the base URL and collects all results", async () => {
    const api = mockApi({ "POST /drafts/accept-batch": acceptAll })
    const onProgress = vi.fn()

    const outcome = await acceptDraftsBatch(ids(60), baseUrl, onProgress)

    expect(draftBatchChunkSize).toBe(25)
    const calls = api.callsTo("POST", "/drafts/accept-batch")
    expect(sentIds(calls)).toEqual([ids(60).slice(0, 25), ids(60).slice(25, 50), ids(60).slice(50)])
    expect(calls.every((call) => (call.body as { baseUrl: string }).baseUrl === baseUrl)).toBe(true)
    expect(outcome.error).toBeUndefined()
    expect(outcome.results.map((r) => r.id)).toEqual(ids(60))
    expect(onProgress.mock.calls).toEqual([[25], [50], [60]])
  })

  it("sends one chunk only after the previous one answered", async () => {
    let open = 0
    let maxOpen = 0
    const resolvers: (() => void)[] = []
    const api = mockApi({
      "POST /drafts/accept-batch": async (call: ApiCall) => {
        open += 1
        maxOpen = Math.max(maxOpen, open)
        await new Promise<void>((resolve) => resolvers.push(resolve))
        open -= 1
        return acceptAll(call)
      },
    })

    const done = acceptDraftsBatch(ids(30), baseUrl)
    await vi.waitFor(() => expect(resolvers).toHaveLength(1))
    expect(api.calls).toHaveLength(1)
    resolvers[0]()
    await vi.waitFor(() => expect(resolvers).toHaveLength(2))
    resolvers[1]()

    expect((await done).results).toHaveLength(30)
    expect(maxOpen).toBe(1)
  })

  it("passes failed and already accepted drafts through as results", async () => {
    mockApi({
      "POST /drafts/accept-batch": {
        results: [
          { id: "d1", status: "accepted" },
          { id: "d2", status: "already_accepted" },
          { id: "d3", status: "failed", code: "parent_missing", error: "no category" },
        ],
      },
    })

    const outcome = await acceptDraftsBatch(ids(3), baseUrl)

    expect(outcome.error).toBeUndefined()
    expect(outcome.results.map((r) => [r.id, r.status, r.code])).toEqual([
      ["d1", "accepted", undefined],
      ["d2", "already_accepted", undefined],
      ["d3", "failed", "parent_missing"],
    ])
  })

  it("keeps the results before a failed request, stops there and returns the error", async () => {
    let request = 0
    const api = mockApi({
      "POST /drafts/accept-batch": (call: ApiCall) => (++request === 2 ? apiError(500, "database gone") : acceptAll(call)),
    })
    const onProgress = vi.fn()

    const outcome = await acceptDraftsBatch(ids(60), baseUrl, onProgress)

    expect(api.callsTo("POST", "/drafts/accept-batch")).toHaveLength(2)
    expect(outcome.results.map((r) => r.id)).toEqual(ids(25))
    expect(outcome.error).toBeInstanceOf(ApiError)
    expect(outcome.error?.message).toBe("database gone")
    expect(onProgress.mock.calls).toEqual([[25]])
  })

  it("returns the error of a failing first request without results", async () => {
    mockApi({ "POST /drafts/accept-batch": apiError(403, "forbidden") })

    await expect(acceptDraftsBatch(ids(2), baseUrl)).resolves.toMatchObject({ results: [], error: { message: "forbidden" } })
  })

  it("sends nothing for an empty list", async () => {
    const api = mockApi({ "POST /drafts/accept-batch": acceptAll })
    const onProgress = vi.fn()

    expect(await acceptDraftsBatch([], baseUrl, onProgress)).toEqual({ results: [] })
    expect(api.calls).toHaveLength(0)
    expect(onProgress).not.toHaveBeenCalled()
  })
})
