import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { acceptDraft, listDrafts, rejectDraft, updateDraft } from "@/lib/drafts-repository"
import { useCapabilities } from "@/hooks/use-capabilities"
import { usePaperlessConfig } from "@/hooks/use-paperless"
import type { DraftAcceptInput, DraftUpdateInput } from "@/types/drafts"

const pendingDraftsKey = ["drafts", "pending"] as const

// The inbox is only fed by the paperless pipeline so far, which needs both
// the LLM and a paperless connection.
export function useInboxAvailable() {
  const { llm } = useCapabilities()
  const { data: config } = usePaperlessConfig()
  return llm && (config?.configured ?? false)
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
