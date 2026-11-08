import { useMutation } from "@tanstack/react-query"
import { useCapabilities } from "@/hooks/use-capabilities"
import { useModules } from "@/hooks/use-modules"
import { quickAdd } from "@/lib/quick-add-repository"
import { draftEntityTypes, draftTargetModule } from "@/components/drafts/draft-input"
import type { DraftEntityType } from "@/types/drafts"

// useQuickAddTypes lists the entity types quick add can create; empty when
// the LLM is off, which hides quick add.
export function useQuickAddTypes(): DraftEntityType[] {
  const { llm } = useCapabilities()
  const { isEnabled } = useModules()
  return llm ? draftEntityTypes.filter((type) => isEnabled(draftTargetModule[type])) : []
}

export function useQuickAdd() {
  return useMutation({ mutationFn: quickAdd })
}
