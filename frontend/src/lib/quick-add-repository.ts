import { post } from "@/lib/api"
import type { QuickAddRequest, QuickAddResponse } from "@/types/quick-add"

export async function quickAdd(input: QuickAddRequest): Promise<QuickAddResponse> {
  return post<QuickAddResponse>("/quick-add", input)
}
