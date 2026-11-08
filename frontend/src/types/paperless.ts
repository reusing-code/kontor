import { z } from "zod/v4"

export type PaperlessEntityType = "contract" | "purchase" | "vehicle" | "cost" | "transaction"

export interface PaperlessConfig {
  configured: boolean
  baseUrl?: string
}

export const paperlessConfigInputSchema = z.object({
  baseUrl: z.string().url(),
  token: z.string().optional(),
})

export type PaperlessConfigInput = z.infer<typeof paperlessConfigInputSchema>

export interface PaperlessSearchResult {
  id: number
  title: string
  created: string
  snippet?: string
}

export interface PaperlessSearchPage {
  count: number
  page: number
  pageSize: number
  results: PaperlessSearchResult[]
}

export interface PaperlessLink {
  entityType: PaperlessEntityType
  entityId: string
  documentId: number
  title: string
  entityUrl: string
  createdAt: string
}

export interface PaperlessAttachResponse {
  links: PaperlessLink[]
  warnings: string[]
}

export interface PaperlessPipelineSettings {
  enabled: boolean
  inboxTag: string
  doneTag: string
}

export interface PaperlessPipelineRunResult {
  found: number
  created: number
  skipped: number
  failed: number
}
