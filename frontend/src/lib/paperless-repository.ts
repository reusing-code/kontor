import { get, put, post, del, getBlob } from "@/lib/api"
import type {
  PaperlessAttachResponse,
  PaperlessConfig,
  PaperlessConfigInput,
  PaperlessEntityType,
  PaperlessLink,
  PaperlessPipelineRunResult,
  PaperlessPipelineSettings,
  PaperlessSearchPage,
} from "@/types/paperless"

export async function getPaperlessConfig(): Promise<PaperlessConfig> {
  return get<PaperlessConfig>("/paperless/config")
}

export async function updatePaperlessConfig(input: PaperlessConfigInput): Promise<PaperlessConfig> {
  return put<PaperlessConfig>("/paperless/config", input)
}

export async function deletePaperlessConfig(): Promise<void> {
  return del("/paperless/config")
}

export async function testPaperlessConfig(): Promise<{ ok: boolean }> {
  return post<{ ok: boolean }>("/paperless/config/test", {})
}

export async function searchPaperlessDocuments(query: string, page: number): Promise<PaperlessSearchPage> {
  const params = new URLSearchParams({ page: String(page) })
  if (query.trim() !== "") {
    params.set("query", query)
  }
  return get<PaperlessSearchPage>(`/paperless/search?${params.toString()}`)
}

export async function getPaperlessThumbnail(documentId: number): Promise<Blob> {
  return getBlob(`/paperless/documents/${documentId}/thumb`)
}

export async function listPaperlessLinks(entityType: PaperlessEntityType, entityId: string): Promise<PaperlessLink[]> {
  return get<PaperlessLink[]>(`/paperless/links/${entityType}/${entityId}`)
}

export async function attachPaperlessDocuments(
  entityType: PaperlessEntityType,
  entityId: string,
  entityUrl: string,
  documents: { id: number; title: string }[],
): Promise<PaperlessAttachResponse> {
  return post<PaperlessAttachResponse>(`/paperless/links/${entityType}/${entityId}`, { entityUrl, documents })
}

export async function detachPaperlessDocument(
  entityType: PaperlessEntityType,
  entityId: string,
  documentId: number,
): Promise<void> {
  return del(`/paperless/links/${entityType}/${entityId}/${documentId}`)
}

export async function getPaperlessPipelineSettings(): Promise<PaperlessPipelineSettings> {
  return get<PaperlessPipelineSettings>("/paperless/pipeline")
}

export async function updatePaperlessPipelineSettings(input: PaperlessPipelineSettings): Promise<PaperlessPipelineSettings> {
  return put<PaperlessPipelineSettings>("/paperless/pipeline", input)
}

export async function runPaperlessPipeline(): Promise<PaperlessPipelineRunResult> {
  return post<PaperlessPipelineRunResult>("/paperless/pipeline/run", {})
}
