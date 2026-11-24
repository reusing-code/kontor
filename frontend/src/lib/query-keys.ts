import type { CategoryKind } from "@/types/category"
import type { PaperlessEntityType } from "@/types/paperless"

// Every key starts with its module or resource, so invalidating a factory's
// `all` (or any shorter prefix) refreshes everything below it.

export function categoryKeys(kind: CategoryKind) {
  const all = ["categories", kind] as const
  return {
    all,
    list: () => [...all, "list"] as const,
    detail: (id: string) => [...all, "detail", id] as const,
  }
}

// Keys of the items filed under categories (contracts, purchases).
export function categoryItemKeys<Root extends string>(root: Root) {
  const all = [root] as const
  return {
    all,
    lists: () => [...all, "list"] as const,
    byCategory: (categoryId: string) => [...all, "list", "category", categoryId] as const,
    detail: (id: string) => [...all, "detail", id] as const,
    summary: () => [...all, "summary"] as const,
  }
}

export type CategoryItemKeys = ReturnType<typeof categoryItemKeys>

export const settingsKeys = { all: ["settings"] } as const

export const capabilitiesKeys = { all: ["capabilities"] } as const

export const authKeys = { config: () => ["auth", "config"] as const } as const

export const tokenKeys = { all: ["tokens"] } as const

const drafts = ["drafts"] as const
export const draftKeys = {
  all: drafts,
  summary: () => [...drafts, "summary"] as const,
  pending: () => [...drafts, "pending"] as const,
}

const paperless = ["paperless"] as const
export const paperlessKeys = {
  all: paperless,
  config: () => [...paperless, "config"] as const,
  pipeline: () => [...paperless, "pipeline"] as const,
  links: () => [...paperless, "links"] as const,
  // entityLinks extends typeLinks, so invalidating a type refreshes the list
  // indicators and every entity's own links.
  typeLinks: (entityType: PaperlessEntityType) => [...paperless, "links", entityType] as const,
  entityLinks: (entityType: PaperlessEntityType, entityId: string) => [...paperless, "links", entityType, entityId] as const,
  search: (query: string, page: number) => [...paperless, "search", query, page] as const,
  thumbnail: (documentId: number) => [...paperless, "thumb", documentId] as const,
}
