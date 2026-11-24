import { toast } from "sonner"
import type { ImportResult } from "@/lib/settings-repository"

export function showImportWarnings(result: ImportResult) {
  for (const warning of result.warnings ?? []) {
    toast.warning(warning)
  }
}
