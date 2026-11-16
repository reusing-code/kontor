import { get, post, put } from "@/lib/api"
import type {
  Draft,
  DraftAcceptInput,
  DraftBatchResponse,
  DraftBatchResult,
  DraftDecisionResponse,
  DraftStatus,
  DraftSummary,
  DraftUpdateInput,
} from "@/types/drafts"

export async function listDrafts(status: DraftStatus | "all" = "pending"): Promise<Draft[]> {
  return get<Draft[]>(`/drafts?status=${status}`)
}

export async function getDraftSummary(): Promise<DraftSummary> {
  return get<DraftSummary>("/drafts/summary")
}

export async function updateDraft(id: string, input: DraftUpdateInput): Promise<Draft> {
  return put<Draft>(`/drafts/${id}`, input)
}

export async function acceptDraft(id: string, input: DraftAcceptInput): Promise<DraftDecisionResponse> {
  return post<DraftDecisionResponse>(`/drafts/${id}/accept`, input)
}

export async function rejectDraft(id: string): Promise<DraftDecisionResponse> {
  return post<DraftDecisionResponse>(`/drafts/${id}/reject`, {})
}

// Drafts per batch request; the backend takes up to 100, smaller requests
// stay short when the source hooks talk to paperless and give progress.
export const draftBatchChunkSize = 25

export interface DraftBatchOutcome {
  results: DraftBatchResult[]
  // Set when a request failed; the drafts without a result were not processed.
  error?: Error
}

// acceptDraftsBatch creates the entities of the drafts on the server and
// accepts them, chunk by chunk. It never throws: results gathered before a
// failing request are kept, since those drafts are already accepted.
export async function acceptDraftsBatch(
  ids: string[],
  baseUrl: string,
  onProgress?: (done: number) => void,
): Promise<DraftBatchOutcome> {
  const results: DraftBatchResult[] = []
  for (let start = 0; start < ids.length; start += draftBatchChunkSize) {
    try {
      const res = await post<DraftBatchResponse>("/drafts/accept-batch", {
        ids: ids.slice(start, start + draftBatchChunkSize),
        baseUrl,
      })
      results.push(...res.results)
    } catch (err) {
      return { results, error: err as Error }
    }
    onProgress?.(Math.min(start + draftBatchChunkSize, ids.length))
  }
  return { results }
}
