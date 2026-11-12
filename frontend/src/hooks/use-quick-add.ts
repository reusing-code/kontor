import { useMutation } from "@tanstack/react-query"
import { useCapabilities } from "@/hooks/use-capabilities"
import { useModules } from "@/hooks/use-modules"
import { quickAdd } from "@/lib/quick-add-repository"
import { classifiedEntityTypes, draftTargetModule } from "@/components/drafts/draft-input"
import type { ClassifiedEntityType } from "@/types/drafts"

// useQuickAddTypes lists the entity types quick add can create; empty when
// the LLM is off, which hides quick add.
export function useQuickAddTypes(): ClassifiedEntityType[] {
  const { llm } = useCapabilities()
  const { isEnabled } = useModules()
  return llm ? classifiedEntityTypes.filter((type) => isEnabled(draftTargetModule[type])) : []
}

export function useQuickAdd() {
  return useMutation({ mutationFn: quickAdd })
}
