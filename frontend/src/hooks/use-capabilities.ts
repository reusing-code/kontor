import { queryOptions, useQuery } from "@tanstack/react-query"
import { getCapabilities } from "@/lib/capabilities-repository"
import { capabilitiesKeys } from "@/lib/query-keys"

// Capabilities are server configuration and only change with a restart.
export const capabilitiesQueryOptions = queryOptions({
  queryKey: capabilitiesKeys.all,
  queryFn: getCapabilities,
  staleTime: Infinity,
})

export function useCapabilities() {
  const { data, isLoading } = useQuery(capabilitiesQueryOptions)
  return {
    llm: data?.llm ?? false,
    llmVision: data?.llmVision ?? false,
    bankImport: data?.bankImport ?? false,
    isLoading,
  }
}
