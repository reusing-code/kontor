import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { acceptDraft, getDraftSummary, listDrafts, rejectDraft, updateDraft } from "@/lib/drafts-repository"
import { useCapabilities } from "@/hooks/use-capabilities"
import { usePaperlessConfig } from "@/hooks/use-paperless"
import type { DraftAcceptInput, DraftUpdateInput } from "@/types/drafts"

const pendingDraftsKey = ["drafts", "pending"] as const

// Counts of pending and of all drafts; invalidated with every ["drafts"] change.
export const draftSummaryQueryOptions = queryOptions({
  queryKey: ["drafts", "summary"],
  queryFn: getDraftSummary,
  refetchInterval: 60 * 1000,
})

// The inbox is available when the paperless pipeline can feed it (the LLM and
// a paperless connection) or when drafts exist, e.g. created by an agent
// through MCP, so they are never hidden.
export function useInboxAvailable() {
  const { llm } = useCapabilities()
  const { data: config } = usePaperlessConfig()
  const { data: summary } = useQuery(draftSummaryQueryOptions)
  return (llm && (config?.configured ?? false)) || (summary?.total ?? 0) > 0
}

export function usePendingDrafts(enabled: boolean) {
  return useQuery({
    queryKey: pendingDraftsKey,
    queryFn: () => listDrafts("pending"),
    enabled,
    refetchInterval: 60 * 1000,
  })
}

export function useUpdateDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: DraftUpdateInput }) => updateDraft(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["drafts"] }),
  })
}

export function useAcceptDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: DraftAcceptInput }) => acceptDraft(id, input),
    onSettled: () => qc.invalidateQueries({ queryKey: ["drafts"] }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["paperless", "links"] }),
  })
}

export function useRejectDraft() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => rejectDraft(id),
    onSettled: () => qc.invalidateQueries({ queryKey: ["drafts"] }),
  })
}
