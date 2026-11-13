import { queryOptions, useQuery } from "@tanstack/react-query"
import { getCapabilities } from "@/lib/capabilities-repository"

// Capabilities are server configuration and only change with a restart.
export const capabilitiesQueryOptions = queryOptions({
  queryKey: ["capabilities"],
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
