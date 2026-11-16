export type DraftStatus = "pending" | "accepted" | "rejected"
export type DraftTargetType =
  | "purchase"
  | "contract"
  | "cost"
  | "vehicle"
  | "contract_category"
  | "purchase_category"
  | "ledger_category"
  | "ledger_account"
  | "none"
export type DraftEntityType = Exclude<DraftTargetType, "none">
// The types the LLM classifier (paperless pipeline, quick add) can propose.
export type ClassifiedEntityType = "purchase" | "contract" | "cost"
export type DraftSourceKind = "paperless" | "ledger" | "email" | "text" | "mcp"

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

export interface DraftSummary {
  pending: number
  total: number
}

export type DraftBatchFailureCode =
  | "not_found"
  | "rejected"
  | "no_type"
  | "module_disabled"
  | "parent_missing"
  | "parent_not_found"
  | "invalid_input"
  | "conflict"
  | "source_link_failed"
  | "internal_error"

// One draft of a batch accept. Failed drafts stay pending.
export interface DraftBatchResult {
  id: string
  status: "accepted" | "already_accepted" | "failed"
  code?: DraftBatchFailureCode
  // Message of the backend (English), set for failed drafts.
  error?: string
  entityType?: DraftEntityType
  entityId?: string
  warnings?: string[]
}

export interface DraftBatchResponse {
  results: DraftBatchResult[]
}
