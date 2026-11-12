import { del, get, post } from "@/lib/api"
import type { Token, TokenCreated, TokenCreateInput } from "@/types/token"

export function listTokens(): Promise<Token[]> {
  return get<Token[]>("/tokens")
}

export function createToken(input: TokenCreateInput): Promise<TokenCreated> {
  return post<TokenCreated>("/tokens", input)
}

export function revokeToken(id: string): Promise<void> {
  return del(`/tokens/${id}`)
}
