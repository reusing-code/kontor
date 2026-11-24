import { keepPreviousData, queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import i18n from "@/i18n"
import {
  attachPaperlessDocuments,
  deletePaperlessConfig,
  detachPaperlessDocument,
  getPaperlessConfig,
  getPaperlessPipelineSettings,
  getPaperlessThumbnail,
  listPaperlessLinks,
  runPaperlessPipeline,
  searchPaperlessDocuments,
  testPaperlessConfig,
  updatePaperlessConfig,
  updatePaperlessPipelineSettings,
} from "@/lib/paperless-repository"
import { draftKeys, paperlessKeys } from "@/lib/query-keys"
import type { PaperlessConfigInput, PaperlessEntityType, PaperlessPipelineSettings } from "@/types/paperless"

export const paperlessConfigQueryOptions = queryOptions({
  queryKey: paperlessKeys.config(),
  queryFn: getPaperlessConfig,
  staleTime: 5 * 60 * 1000,
})

export function usePaperlessConfig() {
  return useQuery(paperlessConfigQueryOptions)
}

export function useUpdatePaperlessConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: PaperlessConfigInput) => updatePaperlessConfig(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: paperlessKeys.all }),
  })
}

export function useDeletePaperlessConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deletePaperlessConfig,
    onSuccess: () => qc.invalidateQueries({ queryKey: paperlessKeys.all }),
  })
}

export function useTestPaperlessConfig() {
  return useMutation({
    mutationFn: testPaperlessConfig,
    meta: { errorMessage: (e) => i18n.t("paperless.testFailed", { message: e.message }) },
  })
}

export function usePaperlessPipelineSettings(enabled: boolean) {
  return useQuery({
    queryKey: paperlessKeys.pipeline(),
    queryFn: getPaperlessPipelineSettings,
    enabled,
  })
}

export function useUpdatePaperlessPipelineSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: PaperlessPipelineSettings) => updatePaperlessPipelineSettings(input),
    onSuccess: (data) => qc.setQueryData(paperlessKeys.pipeline(), data),
  })
}

export function useRunPaperlessPipeline() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: runPaperlessPipeline,
    onSuccess: () => qc.invalidateQueries({ queryKey: draftKeys.all }),
  })
}

export function usePaperlessLinks(entityType: PaperlessEntityType, entityId: string, enabled: boolean) {
  return useQuery({
    queryKey: paperlessKeys.entityLinks(entityType, entityId),
    queryFn: () => listPaperlessLinks(entityType, entityId),
    enabled,
  })
}

export function usePaperlessSearch(query: string, page: number, enabled: boolean) {
  return useQuery({
    queryKey: paperlessKeys.search(query, page),
    queryFn: () => searchPaperlessDocuments(query, page),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 30 * 1000,
  })
}

export function useAttachPaperlessDocuments(entityType: PaperlessEntityType, entityId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ entityUrl, documents }: { entityUrl: string; documents: { id: number; title: string }[] }) =>
      attachPaperlessDocuments(entityType, entityId, entityUrl, documents),
    onSuccess: () => qc.invalidateQueries({ queryKey: paperlessKeys.entityLinks(entityType, entityId) }),
  })
}

export function useDetachPaperlessDocument(entityType: PaperlessEntityType, entityId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentId: number) => detachPaperlessDocument(entityType, entityId, documentId),
    onSuccess: () => qc.invalidateQueries({ queryKey: paperlessKeys.entityLinks(entityType, entityId) }),
  })
}

// usePaperlessThumbnail loads the image through the authenticated API (an
// <img src> cannot send the Bearer header) and hands back an object URL. The
// URL lives in the query cache and stays valid for the page session.
export function usePaperlessThumbnail(documentId: number, enabled: boolean) {
  const { data: url } = useQuery({
    queryKey: paperlessKeys.thumbnail(documentId),
    queryFn: async () => URL.createObjectURL(await getPaperlessThumbnail(documentId)),
    enabled,
    staleTime: Infinity,
  })
  return url ?? null
}
