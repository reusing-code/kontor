export type DraftStatus = "pending" | "accepted" | "rejected"
export type DraftTargetType = "purchase" | "contract" | "cost" | "none"
export type DraftEntityType = Exclude<DraftTargetType, "none">
export type DraftSourceKind = "paperless" | "email" | "text" | "mcp"

export interface DraftSource {
  kind: DraftSourceKind
  ref: string
  title?: string
  url?: string
}

export interface Draft {
  id: string
  status: DraftStatus
  targetType: DraftTargetType
  parentId?: string
  // Shape of the target module's create input.
  input: Record<string, unknown>
  source: DraftSource
  // Machine-readable code, translated via drafts.notes.*
  note?: string
  result?: { entityType: DraftEntityType; entityId: string }
  createdAt: string
  updatedAt: string
  decidedAt?: string
}

export interface DraftUpdateInput {
  targetType: DraftTargetType
  parentId: string
  input: Record<string, unknown>
}

export interface DraftAcceptInput {
  entityType: DraftEntityType
  entityId: string
  entityUrl: string
}

export interface DraftDecisionResponse {
  draft: Draft
  warnings: string[]
}
