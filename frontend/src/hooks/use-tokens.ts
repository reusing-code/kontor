import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { createToken, listTokens, revokeToken } from "@/lib/token-repository"
import type { TokenCreateInput } from "@/types/token"

export const tokensQueryOptions = queryOptions({
  queryKey: ["tokens"],
  queryFn: listTokens,
})

export function useTokens() {
  return useQuery(tokensQueryOptions)
}

// The create response carries the raw token: callers keep it in component
// state and it must never be written to the ["tokens"] query cache, which
// only holds the list without secrets.
export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: TokenCreateInput) => createToken(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tokens"] }),
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => revokeToken(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["tokens"] }),
  })
}
