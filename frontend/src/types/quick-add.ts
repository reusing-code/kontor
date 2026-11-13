import type { ClassifiedEntityType, DraftTargetType } from "@/types/drafts"

export const quickAddMaxLength = 2000
export const quickAddMaxFileBytes = 10 * 1024 * 1024

export interface QuickAddRequest {
  // The note; with a file an optional hint about it.
  text: string
  // A PDF, or an image when the llmVision capability is on.
  file?: File
  // Forces the entity type, e.g. after the user picked another type.
  targetType?: ClassifiedEntityType
}

// A non-EUR amount; converted to EUR at the ECB rate when rate is set.
export interface QuickAddCurrency {
  code: string
  amount: number
  converted: boolean
  rate?: number
  rateDate?: string
}

export interface QuickAddResponse {
  targetType: DraftTargetType
  parentId?: string
  // Shape of the target module's create input.
  input: Record<string, unknown>
  confident: boolean
  // Machine-readable code, translated via quickAdd.notes.*
  note?: string
  currency?: QuickAddCurrency
}
