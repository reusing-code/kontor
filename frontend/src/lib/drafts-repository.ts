import { get, post, put } from "@/lib/api"
import type { Draft, DraftAcceptInput, DraftDecisionResponse, DraftStatus, DraftSummary, DraftUpdateInput } from "@/types/drafts"

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
