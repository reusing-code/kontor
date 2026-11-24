import { describe, expect, it } from "vitest"
import { act, renderHook, waitFor } from "@testing-library/react"
import { useAttachPaperlessDocuments, useDetachPaperlessDocument, usePaperlessLinkIndex } from "@/hooks/use-paperless"
import { mockApi } from "@/test/api-mock"
import { createTestQueryClient, queryWrapper } from "@/test/render"
import type { PaperlessLink } from "@/types/paperless"

const at = "2026-11-01T10:00:00Z"

function link(entityId: string, documentId: number): PaperlessLink {
  return { entityType: "cost", entityId, documentId, title: `Doc ${documentId}`, entityUrl: "https://kontor.example/x", createdAt: at }
}

function setup() {
  let stored = [link("cost-1", 1), link("cost-1", 2), link("cost-2", 3)]
  const api = mockApi({
    "GET /paperless/config": { configured: true },
    "GET /paperless/links/cost": () => stored,
    "POST /paperless/links/cost/cost-3": () => {
      stored = [...stored, link("cost-3", 4)]
      return { links: [link("cost-3", 4)], warnings: [] }
    },
    "DELETE /paperless/links/cost/cost-2/3": () => {
      stored = stored.filter((l) => l.documentId !== 3)
      return undefined
    },
  })
  const { result } = renderHook(
    () => ({
      index: usePaperlessLinkIndex("cost"),
      attach: useAttachPaperlessDocuments("cost", "cost-3"),
      detach: useDetachPaperlessDocument("cost", "cost-2"),
    }),
    { wrapper: queryWrapper(createTestQueryClient()) },
  )
  return { api, result }
}

describe("usePaperlessLinkIndex", () => {
  it("groups the links of one type by entity", async () => {
    const { result } = setup()
    await waitFor(() => expect(result.current.index).toBeDefined())
    expect(result.current.index?.get("cost-1")?.map((l) => l.documentId)).toEqual([1, 2])
    expect(result.current.index?.get("cost-2")?.map((l) => l.documentId)).toEqual([3])
    expect(result.current.index?.get("cost-3")).toBeUndefined()
  })

  it("refreshes after attach and detach", async () => {
    const { api, result } = setup()
    await waitFor(() => expect(result.current.index).toBeDefined())

    await act(() => result.current.attach.mutateAsync({ entityUrl: "https://kontor.example/x", documents: [{ id: 4, title: "Doc 4" }] }))
    await waitFor(() => expect(result.current.index?.get("cost-3")?.map((l) => l.documentId)).toEqual([4]))

    await act(() => result.current.detach.mutateAsync(3))
    await waitFor(() => expect(result.current.index?.get("cost-2")).toBeUndefined())

    expect(api.callsTo("GET", "/paperless/links/cost")).toHaveLength(3)
  })
})
