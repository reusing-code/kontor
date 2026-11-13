import { post, postForm } from "@/lib/api"
import type { QuickAddRequest, QuickAddResponse } from "@/types/quick-add"

// A typed note goes as JSON; with a file the request is multipart and the
// text is an optional hint.
export async function quickAdd({ text, file, targetType }: QuickAddRequest): Promise<QuickAddResponse> {
  if (!file) return post<QuickAddResponse>("/quick-add", { text, targetType })
  const form = new FormData()
  form.append("file", file)
  if (text) form.append("text", text)
  if (targetType) form.append("targetType", targetType)
  return postForm<QuickAddResponse>("/quick-add", form)
}
