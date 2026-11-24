import { queryOptions, useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { createToken, listTokens, revokeToken } from "@/lib/token-repository"
import { tokenKeys } from "@/lib/query-keys"
import type { TokenCreateInput } from "@/types/token"

export const tokensQueryOptions = queryOptions({
  queryKey: tokenKeys.all,
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
    // the response holds the raw token: drop it from the mutation cache as
    // soon as the card resets the mutation instead of after the default 5 minutes
    gcTime: 0,
    onSuccess: () => qc.invalidateQueries({ queryKey: tokenKeys.all }),
  })
}

export function useRevokeToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => revokeToken(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: tokenKeys.all }),
  })
}
